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
	// DummyVerify burns the work a real Verify would, against a hash that
	// cannot match. The login handler calls it for an unknown username, so an
	// implementation that skipped the work would reintroduce the timing oracle
	// it exists to close — which is why it belongs in the interface rather
	// than only on the concrete type.
	DummyVerify(password string)
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
// the production hasher: it works at pbkdf2Iterations.
type PBKDF2Hasher struct {
	// Iterations overrides the work factor Hash writes, and the floor Verify
	// measures a stored hash against. Zero — the only value production ever
	// uses — means pbkdf2Iterations.
	//
	// It exists for tests, and specifically for the API integration suite,
	// which creates and signs in as ~50 accounts. At the real work factor that
	// is ~265 deliberately-slow hashes and about 80% of the suite's runtime,
	// spent proving nothing about hashing — every one of those tests is about
	// something else, and needs a password only to get through the door.
	// internal/auth's own tests leave this zero, so the shipped work factor is
	// still exercised at full cost by the package that owns it.
	Iterations int
}

// compile-time check that the implementation satisfies the interface.
var _ Hasher = PBKDF2Hasher{}

// iterations is the work factor this hasher writes. Read through here rather
// than off the field directly, so "unset means production" is stated once.
func (h PBKDF2Hasher) iterations() int {
	if h.Iterations <= 0 {
		return pbkdf2Iterations
	}
	return h.Iterations
}

// Hash produces "$pbkdf2-sha256$i=<iters>$<salt-b64>$<key-b64>" with a fresh
// random salt.
func (h PBKDF2Hasher) Hash(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("%w: %v", ErrHashFailed, err)
	}
	iters := h.iterations()
	key, err := pbkdf2Key(password, salt, iters, keyLen)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrHashFailed, err)
	}
	return encodePHC(algPBKDF2, iters, salt, key), nil
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
func (h PBKDF2Hasher) Verify(password, encoded string) (ok, needsRehash bool) {
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
	return true, iters < h.iterations() || len(salt) < saltLen || len(want) < keyLen
}

// DummyVerify burns roughly the same CPU as a real Verify against a hash that
// cannot match. Login calls it when the username is unknown, so the response
// time does not reveal whether an account exists.
func (h PBKDF2Hasher) DummyVerify(password string) {
	h.Verify(password, h.dummyHash())
}

// dummyHash is a syntactically valid PHC string over a fixed salt. Its plaintext
// is unknown and irrelevant — it exists only to make DummyVerify do the work.
//
// Built at this hasher's own work factor rather than fixed at the shipped one:
// the property DummyVerify is defending is that an unknown username costs the
// same as a known one, and that is a claim about whatever factor Hash is
// currently writing. A constant would hold only while the two happened to
// agree, and would leave a test hasher paying the full price for the one code
// path the tests take most.
// Goes through encodePHC like every other hash, so there is still exactly one
// definition of the stored form — the previous constant spelled it out a second
// time, and a change to the encoding would have left this one behind.
func (h PBKDF2Hasher) dummyHash() string {
	return encodePHC(algPBKDF2, h.iterations(), make([]byte, saltLen), make([]byte, keyLen))
}

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
