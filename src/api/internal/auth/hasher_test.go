package auth

import (
	"strings"
	"testing"
)

// The real work factor makes each Hash/Verify take a noticeable fraction of a
// second, which is the point of it — but a table of twenty cases would then
// dominate the suite's runtime. Tests that only care about the encoding use
// hashWith to build a string at a low iteration count instead.
func hashWith(t *testing.T, password string, iters int) string {
	t.Helper()
	salt := make([]byte, saltLen)
	for i := range salt {
		salt[i] = byte(i)
	}
	key, err := pbkdf2Key(password, salt, iters, keyLen)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	return encodePHC(algPBKDF2, iters, salt, key)
}

func TestHashVerifyRoundTrip(t *testing.T) {
	h := PBKDF2Hasher{}
	encoded, err := h.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	ok, needsRehash := h.Verify("correct horse battery staple", encoded)
	if !ok {
		t.Error("Verify rejected the password it just hashed")
	}
	if needsRehash {
		t.Error("a freshly written hash should not need rehashing")
	}
}

func TestHashIsSaltedPerCall(t *testing.T) {
	h := PBKDF2Hasher{}
	a, err := h.Hash("same password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	b, err := h.Hash("same password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if a == b {
		t.Error("two hashes of the same password are identical — the salt is not random")
	}
}

func TestVerifyRejectsWrongPassword(t *testing.T) {
	h := PBKDF2Hasher{}
	encoded := hashWith(t, "right", 1000)

	if ok, _ := h.Verify("wrong", encoded); ok {
		t.Error("Verify accepted the wrong password")
	}
}

// A stored hash is data, and data can be wrong — corrupted, truncated, or
// written by an attacker with a foothold in the database. None of those may
// verify, and none may panic.
func TestVerifyRejectsMalformedEncodings(t *testing.T) {
	valid := hashWith(t, "pw", 1000)

	tests := map[string]string{
		"empty":                "",
		"no leading dollar":    strings.TrimPrefix(valid, "$"),
		"too few fields":       "$pbkdf2-sha256$i=1000$c2FsdA",
		"too many fields":      valid + "$extra",
		"unknown algorithm":    "$scrypt$i=1000$c2FsdA$a2V5",
		"argon2 not yet built": "$argon2id$i=1000$c2FsdA$a2V5",
		"missing i= prefix":    "$pbkdf2-sha256$1000$c2FsdA$a2V5",
		"non-numeric iters":    "$pbkdf2-sha256$i=lots$c2FsdA$a2V5",
		"zero iters":           "$pbkdf2-sha256$i=0$c2FsdA$a2V5",
		"negative iters":       "$pbkdf2-sha256$i=-1$c2FsdA$a2V5",
		"absurd iters":         "$pbkdf2-sha256$i=999999999$c2FsdA$a2V5",
		"bad base64 salt":      "$pbkdf2-sha256$i=1000$not!base64$a2V5",
		"bad base64 key":       "$pbkdf2-sha256$i=1000$c2FsdA$not!base64",
		"empty salt":           "$pbkdf2-sha256$i=1000$$a2V5",
		"empty key":            "$pbkdf2-sha256$i=1000$c2FsdA$",
	}

	h := PBKDF2Hasher{}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			ok, needsRehash := h.Verify("pw", encoded)
			if ok {
				t.Errorf("Verify(%q) accepted a malformed hash", encoded)
			}
			if needsRehash {
				t.Error("a rejected hash must not also ask to be rehashed")
			}
		})
	}
}

// An out-of-range iteration count must be refused before it reaches the KDF.
// Accepting i=2000000000 would let one unauthenticated request pin a core for
// minutes — a denial of service written into a table row.
func TestVerifyRefusesExcessiveIterationsWithoutComputing(t *testing.T) {
	h := PBKDF2Hasher{}
	// Well above pbkdf2MaxIterations; if this were actually computed the test
	// would hang rather than fail.
	if ok, _ := h.Verify("pw", "$pbkdf2-sha256$i=2000000000$c2FsdA$a2V5"); ok {
		t.Error("Verify accepted an absurd iteration count")
	}
}

func TestVerifyFlagsWeakerStoredParameters(t *testing.T) {
	h := PBKDF2Hasher{}
	// A hash written when the work factor was lower still verifies, but asks
	// to be replaced — that is how users upgrade without a migration.
	encoded := hashWith(t, "pw", 1000)

	ok, needsRehash := h.Verify("pw", encoded)
	if !ok {
		t.Fatal("Verify rejected a valid hash written at a lower work factor")
	}
	if !needsRehash {
		t.Error("a hash below the current iteration count should need rehashing")
	}
}

func TestEncodedFormIsSelfDescribing(t *testing.T) {
	encoded := hashWith(t, "pw", 1000)
	if !strings.HasPrefix(encoded, "$"+algPBKDF2+"$i=1000$") {
		t.Errorf("encoded = %q, want a $%s$i=1000$… prefix", encoded, algPBKDF2)
	}
	// The tag is what lets Verify dispatch, and what lets a future Argon2id
	// hasher coexist with these rows.
	alg, iters, salt, key, err := decodePHC(encoded)
	if err != nil {
		t.Fatalf("decodePHC: %v", err)
	}
	if alg != algPBKDF2 || iters != 1000 || len(salt) != saltLen || len(key) != keyLen {
		t.Errorf("decodePHC = (%q, %d, %d bytes salt, %d bytes key), want (%q, 1000, %d, %d)",
			alg, iters, len(salt), len(key), algPBKDF2, saltLen, keyLen)
	}
}

// The dummy hash exists to spend time on an unknown username. If it stopped
// parsing, DummyVerify would return early and hand back a timing oracle for
// account enumeration — silently, since nothing else depends on it.
func TestDummyHashIsWellFormed(t *testing.T) {
	alg, _, salt, key, err := decodePHC(dummyHash)
	if err != nil {
		t.Fatalf("the dummy hash no longer parses: %v", err)
	}
	if alg != algPBKDF2 {
		t.Errorf("dummy hash algorithm = %q, want %q", alg, algPBKDF2)
	}
	if len(salt) != saltLen || len(key) != keyLen {
		t.Errorf("dummy hash has %d-byte salt and %d-byte key, want %d and %d",
			len(salt), len(key), saltLen, keyLen)
	}
	if ok, _ := (PBKDF2Hasher{}).Verify("anything", dummyHash); ok {
		t.Error("the dummy hash verified against a password")
	}
}
