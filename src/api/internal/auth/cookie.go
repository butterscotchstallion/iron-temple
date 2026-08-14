package auth

import (
	"net/http"
	"time"
)

// CookieName is the session cookie. Prefixed to make its scope obvious in a
// browser's storage inspector alongside cookies from anything else on the host.
const CookieName = "it_session"

// Session lifetimes.
//
// Two tiers, chosen by the "remember me" box at login:
//
//   - Without it, the cookie has no Max-Age (it dies with the browser session)
//     and the server-side row expires in a day. Right for a shared machine.
//   - With it, the cookie lasts 60 days and its expiry slides forward as it is
//     used, so an active user is never signed out by the clock.
//
// The server-side expiry is the authority; the cookie attribute is only a hint
// the browser may ignore. Both are set so a stolen-then-abandoned cookie still
// dies on schedule.
const (
	// SessionTTL is how long a non-persistent login stays valid server-side.
	SessionTTL = 24 * time.Hour
	// PersistentTTL is how long a "remember me" login stays valid, and the
	// window it slides forward by on use.
	PersistentTTL = 60 * 24 * time.Hour
	// SlideAfter is how stale a persistent session must be before its expiry
	// is pushed out again. Without this, every request would write a row.
	SlideAfter = 24 * time.Hour
)

// TTL returns the server-side lifetime for a login of the given kind.
func TTL(persistent bool) time.Duration {
	if persistent {
		return PersistentTTL
	}
	return SessionTTL
}

// SetCookie writes the session cookie for a freshly issued token.
//
// secure should be false only for plain-HTTP local development: a Secure cookie
// is never sent over http://localhost's scheme, so setting it unconditionally
// would make dev logins silently fail to stick.
func SetCookie(w http.ResponseWriter, token string, persistent, secure bool) {
	// #nosec G124 -- Secure is set from the caller's environment rather than a
	// literal, which the rule cannot follow. It is true everywhere except local
	// plain-HTTP development, where a Secure cookie would never be sent back
	// and every dev login would silently fail to stick. HttpOnly and SameSite,
	// the other two attributes the rule checks, are unconditional below.
	c := &http.Cookie{
		Name:  CookieName,
		Value: token,
		Path:  "/",
		// The cookie is read by the API, never by JavaScript — HttpOnly keeps
		// an XSS bug from turning into stolen sessions.
		HttpOnly: true,
		Secure:   secure,
		// Lax is the CSRF defence: the browser withholds this cookie from
		// cross-site POST/PATCH/DELETE while still sending it when the user
		// follows a link in. The UI is same-origin with the API in both dev
		// (Vite proxy) and production (Traefik), so nothing legitimate is lost.
		SameSite: http.SameSiteLaxMode,
	}
	if persistent {
		c.MaxAge = int(PersistentTTL / time.Second)
	}
	// No MaxAge and no Expires makes it a session cookie, cleared on browser
	// close — which is exactly what "don't remember me" should mean.
	http.SetCookie(w, c)
}

// ClearCookie expires the session cookie. The attributes must match those used
// when setting it, or the browser keeps the original alongside the deletion.
func ClearCookie(w http.ResponseWriter, secure bool) {
	// #nosec G124 -- see SetCookie; the attributes must match the ones used to
	// set the cookie or the browser will not treat this as the same cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// TokenFromRequest returns the session token presented by r, if any.
func TokenFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	return c.Value, true
}
