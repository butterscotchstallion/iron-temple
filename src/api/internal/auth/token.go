package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// tokenLen is the raw entropy behind a session cookie, in bytes. 256 bits puts
// guessing out of reach, so sessions need no additional signing.
const tokenLen = 32

// NewToken mints a session token: the value to hand the browser, and the digest
// to store.
//
// Only the digest is persisted. A read of user_sessions — a database dump, a
// log of a query, a backup on the wrong disk — therefore yields nothing that
// can be replayed as a cookie, the same reason passwords are not stored plain.
func NewToken() (token string, digest []byte, err error) {
	raw := make([]byte, tokenLen)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("auth: could not generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, TokenDigest(token), nil
}

// TokenDigest maps a cookie value to its stored form. Lookup hashes the
// presented token and matches on the digest, so this is the only bridge between
// what the browser holds and what the database holds.
//
// A plain SHA-256 is right here where it would be wrong for a password: the
// input is 256 bits of uniform randomness, not a guessable secret, so there is
// nothing for a work factor to slow down.
func TokenDigest(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
