package auth

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTokenIsUniqueAndFullLength(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		token, digest, err := NewToken()
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		if seen[token] {
			t.Fatal("NewToken returned a duplicate — the token is not random")
		}
		seen[token] = true

		raw, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			t.Fatalf("token is not base64url: %v", err)
		}
		if len(raw) != tokenLen {
			t.Errorf("token carries %d bytes of entropy, want %d", len(raw), tokenLen)
		}
		if !bytes.Equal(digest, TokenDigest(token)) {
			t.Error("the returned digest is not the digest of the returned token")
		}
	}
}

// The stored digest must not be reversible to the cookie value — that is the
// whole reason the digest is what gets persisted.
func TestTokenDigestDoesNotContainTheToken(t *testing.T) {
	token, digest, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if bytes.Contains(digest, []byte(token)) {
		t.Error("the digest contains the token verbatim")
	}
	if len(digest) != 32 {
		t.Errorf("digest is %d bytes, want 32 (SHA-256)", len(digest))
	}
}

func TestTokenDigestIsStable(t *testing.T) {
	// Lookup depends on this: the digest computed at login must equal the one
	// computed from the cookie on every later request.
	if !bytes.Equal(TokenDigest("abc"), TokenDigest("abc")) {
		t.Error("TokenDigest is not deterministic")
	}
	if bytes.Equal(TokenDigest("abc"), TokenDigest("abd")) {
		t.Error("TokenDigest collided on different inputs")
	}
}

func TestTTL(t *testing.T) {
	if got := TTL(false); got != SessionTTL {
		t.Errorf("TTL(false) = %v, want %v", got, SessionTTL)
	}
	if got := TTL(true); got != PersistentTTL {
		t.Errorf("TTL(true) = %v, want %v", got, PersistentTTL)
	}
}

// "Remember me" is the user-visible promise here: the cookie has to outlive the
// browser session, and the plain login has to not.
func TestSetCookieLifetimes(t *testing.T) {
	tests := []struct {
		name       string
		persistent bool
		wantMaxAge int
	}{
		{"plain login is a browser-session cookie", false, 0},
		{"remember me lasts 60 days", true, int(PersistentTTL / time.Second)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			SetCookie(rec, "tok", tc.persistent, true)

			c := readCookie(t, rec)
			if c.MaxAge != tc.wantMaxAge {
				t.Errorf("MaxAge = %d, want %d", c.MaxAge, tc.wantMaxAge)
			}
			if c.Value != "tok" {
				t.Errorf("Value = %q, want %q", c.Value, "tok")
			}
		})
	}
}

func TestSetCookieHardeningAttributes(t *testing.T) {
	rec := httptest.NewRecorder()
	SetCookie(rec, "tok", true, true)
	c := readCookie(t, rec)

	if !c.HttpOnly {
		t.Error("cookie is not HttpOnly — script could read the session")
	}
	if !c.Secure {
		t.Error("cookie is not Secure despite secure=true")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax (the CSRF defence)", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q, want /", c.Path)
	}
}

// Plain-HTTP dev: a Secure cookie would never be sent back, so logins would
// appear to succeed and then immediately not stick.
func TestSetCookieOmitsSecureForLocalDevelopment(t *testing.T) {
	rec := httptest.NewRecorder()
	SetCookie(rec, "tok", false, false)
	if readCookie(t, rec).Secure {
		t.Error("cookie is Secure despite secure=false")
	}
}

func TestClearCookieExpiresIt(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearCookie(rec, true)
	c := readCookie(t, rec)

	if c.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want a negative value to delete the cookie", c.MaxAge)
	}
	if c.Value != "" {
		t.Errorf("Value = %q, want empty", c.Value)
	}
	// Attributes must match the ones used to set it, or the browser treats it
	// as a different cookie and keeps the original.
	if c.Path != "/" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("clearing cookie attributes differ from the ones used to set it: %+v", c)
	}
}

func TestTokenFromRequest(t *testing.T) {
	tests := []struct {
		name   string
		cookie *http.Cookie
		want   string
		wantOK bool
	}{
		{"no cookie", nil, "", false},
		{"empty value", &http.Cookie{Name: CookieName, Value: ""}, "", false},
		{"wrong name", &http.Cookie{Name: "other", Value: "tok"}, "", false},
		{"present", &http.Cookie{Name: CookieName, Value: "tok"}, "tok", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.cookie != nil {
				r.AddCookie(tc.cookie)
			}
			got, ok := TokenFromRequest(r)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("TokenFromRequest = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func readCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want exactly 1", len(cookies))
	}
	if cookies[0].Name != CookieName {
		t.Fatalf("cookie name = %q, want %q", cookies[0].Name, CookieName)
	}
	return cookies[0]
}
