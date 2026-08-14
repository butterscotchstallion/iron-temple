// Package auth owns credential handling: password hashing, opaque session
// tokens, the session cookie, and login rate limiting. It has no knowledge of
// HTTP routing or of the database — the api package wires it to both.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Hasher turns a password into a storable string and checks one against it.
//
// The stored form is PHC-style — it names its own algorithm and parameters —
// so the scheme can change without a migration or a flag day: Verify dispatches
// on the tag it finds, and reports needsRehash when the stored parameters are
// weaker than what Hash produces today. Callers re-hash on the next successful
// login, and users upgrade silently as they sign in.
type Hasher interface {
	// Hash returns the PHC-encoded hash of password.
	Hash(password string) (string, error)
	// Verify reports whether password matches encoded, and whether encoded
	// was produced by a weaker parameter set and should be replaced.
	// A malformed or unrecognised encoded value is a non-match, not an error.
	Verify(password, encoded string) (ok, needsRehash bool)
}

// PBKDF2 parameters.
//
// Argon2id would be the stronger choice — it is memory-hard, which PBKDF2 is
// not — but golang.org/x/crypto is vendored here without the argon2 package and
// this module builds offline, so reaching for it would break the build. PBKDF2
// -HMAC-SHA256 at 600k iterations is OWASP's current floor for that algorithm,
// and the PHC encoding above is what makes the eventual swap a one-case change.
const (
	// algPBKDF2 is the tag written into (and matched out of) the PHC string.
	algPBKDF2 = "pbkdf2-sha256"

	// pbkdf2Iterations is the current work factor. Raising it is safe: older
	// hashes keep verifying and are flagged for rehash on next login.
	pbkdf2Iterations = 600_000

	// pbkdf2MaxIterations bounds what Verify will accept out of storage. The
	// value comes from the database, and a tampered row carrying i=2000000000
	// would otherwise turn one login attempt into a CPU exhaustion primitive.
	pbkdf2MaxIterations = 10_000_000

	saltLen = 16
	keyLen  = 32
)

// ErrHashFailed is returned when the system's entropy source is unavailable, so
// a caller cannot mistake an unsalted or empty hash for a usable one.
var ErrHashFailed = errors.New("auth: could not hash password")

// PBKDF2Hasher implements Hasher with PBKDF2-HMAC-SHA256. The zero value is
// ready to use.
type PBKDF2Hasher struct{}

// compile-time check that the implementation satisfies the interface.
var _ Hasher = PBKDF2Hasher{}

// Hash produces "$pbkdf2-sha256$i=<iters>$<salt-b64>$<key-b64>" with a fresh
// random salt.
func (PBKDF2Hasher) Hash(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("%w: %v", ErrHashFailed, err)
	}
	key, err := pbkdf2Key(password, salt, pbkdf2Iterations, keyLen)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrHashFailed, err)
	}
	return encodePHC(algPBKDF2, pbkdf2Iterations, salt, key), nil
}

// pbkdf2Key is the single call site for the KDF, so Hash and Verify cannot
// drift apart on the hash function they use.
func pbkdf2Key(password string, salt []byte, iters, length int) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, iters, length)
}

// Verify checks password against encoded in constant time.
//
// Every failure mode — unknown algorithm, malformed field, out-of-range
// iteration count, wrong password — returns (false, false). A caller cannot
// distinguish "this row is corrupt" from "wrong password", which is the right
// answer for a login endpoint.
func (PBKDF2Hasher) Verify(password, encoded string) (ok, needsRehash bool) {
	alg, iters, salt, want, err := decodePHC(encoded)
	if err != nil || alg != algPBKDF2 {
		return false, false
	}
	got, err := pbkdf2Key(password, salt, iters, len(want))
	if err != nil {
		return false, false
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return false, false
	}
	// Only an authenticated caller learns that their stored parameters are
	// stale, so this leaks nothing to an attacker guessing passwords.
	return true, iters < pbkdf2Iterations || len(salt) < saltLen || len(want) < keyLen
}

// DummyVerify burns roughly the same CPU as a real Verify against a hash that
// cannot match. Login calls it when the username is unknown, so the response
// time does not reveal whether an account exists.
func (h PBKDF2Hasher) DummyVerify(password string) {
	h.Verify(password, dummyHash)
}

// dummyHash is a syntactically valid PHC string over a fixed salt. Its plaintext
// is unknown and irrelevant — it exists only to make DummyVerify do the work.
const dummyHash = "$pbkdf2-sha256$i=600000$AAAAAAAAAAAAAAAAAAAAAA$" +
	"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// b64 is the unpadded encoding PHC strings conventionally use; padding would
// collide with the "=" already used inside the parameter field.
var b64 = base64.RawStdEncoding

func encodePHC(alg string, iters int, salt, key []byte) string {
	return fmt.Sprintf("$%s$i=%d$%s$%s", alg, iters, b64.EncodeToString(salt), b64.EncodeToString(key))
}

// decodePHC splits "$alg$i=N$salt$key" into its parts. It is strict: anything
// it cannot parse exactly is an error rather than a best-effort read.
func decodePHC(encoded string) (alg string, iters int, salt, key []byte, err error) {
	// A leading "$" means Split yields an empty first field; five fields total.
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "" {
		return "", 0, nil, nil, errors.New("auth: malformed hash")
	}
	alg = parts[1]

	params, ok := strings.CutPrefix(parts[2], "i=")
	if !ok {
		return "", 0, nil, nil, errors.New("auth: malformed hash parameters")
	}
	iters, err = strconv.Atoi(params)
	if err != nil || iters < 1 || iters > pbkdf2MaxIterations {
		return "", 0, nil, nil, errors.New("auth: iteration count out of range")
	}

	if salt, err = b64.DecodeString(parts[3]); err != nil || len(salt) == 0 {
		return "", 0, nil, nil, errors.New("auth: malformed salt")
	}
	if key, err = b64.DecodeString(parts[4]); err != nil || len(key) == 0 {
		return "", 0, nil, nil, errors.New("auth: malformed key")
	}
	return alg, iters, salt, key, nil
}
