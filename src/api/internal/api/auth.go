package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/auth"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// currentUser is the authenticated caller, carried on the request context by
// requireUser so handlers never re-read the cookie or re-query the session.
type currentUser struct {
	ID          int32
	Username    string
	DisplayName string
	AvatarColor string
	IsAdmin     bool
	// tokenHash identifies *this* login among the user's sessions, so logout
	// can revoke exactly the one presented and a password change can revoke
	// every other one.
	tokenHash []byte
}

// ctxKey is unexported so nothing outside this package can write a fake user
// onto a context and walk past the middleware.
type ctxKey struct{}

var userKey ctxKey

// userFrom returns the authenticated caller. It panics when no user is present,
// because that can only mean a handler was mounted outside requireUser — a
// wiring bug that must fail loudly in tests rather than quietly serve one user's
// data to another. Handlers reachable both with and without a session use
// optionalUserFrom instead.
func userFrom(ctx context.Context) currentUser {
	u, ok := ctx.Value(userKey).(currentUser)
	if !ok {
		panic("api: handler requires an authenticated user but is not mounted under requireUser")
	}
	return u
}

// requireUser authenticates the session cookie and rejects the request if it is
// missing, unknown, or expired.
func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.authenticate(w, r)
		if !ok {
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

// authenticate resolves the cookie to a user, writing the 401 itself when it
// cannot. Returns ok=false if the caller should stop.
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (currentUser, bool) {
	token, ok := auth.TokenFromRequest(r)
	if !ok {
		unauthorized(w, "authentication required")
		return currentUser{}, false
	}

	ctx := r.Context()
	digest := auth.TokenDigest(token)
	row, err := s.q.GetUserSession(ctx, digest)
	if errors.Is(err, pgx.ErrNoRows) {
		// Unknown or expired. Clear the cookie so the browser stops presenting
		// it — otherwise the UI would 401 on every request forever, with no way
		// for the user to recover short of clearing site data by hand.
		auth.ClearCookie(w, s.secureCookies())
		unauthorized(w, "session expired")
		return currentUser{}, false
	}
	if err != nil {
		internalError(w)
		return currentUser{}, false
	}

	s.slideSession(ctx, row)

	return currentUser{
		ID:          row.UserID,
		Username:    row.Username,
		DisplayName: row.DisplayName,
		AvatarColor: row.AvatarColor,
		IsAdmin:     row.IsAdmin,
		tokenHash:   digest,
	}, true
}

// slideSession pushes a "remember me" session's expiry forward once a day of
// use has passed, which is what keeps an active user signed in indefinitely.
// Non-persistent sessions are left to expire on schedule — a 24-hour login that
// renewed itself would be a permanent one under another name.
//
// Failures are ignored: the request is already authenticated, and refusing to
// serve it because a bookkeeping write failed would turn a transient database
// hiccup into a logout.
func (s *Server) slideSession(ctx context.Context, row store.GetUserSessionRow) {
	if !row.Persistent || !row.LastSeen.Valid {
		return
	}
	if time.Since(row.LastSeen.Time) < auth.SlideAfter {
		return
	}
	_ = s.q.TouchUserSession(ctx, store.TouchUserSessionParams{
		TtlSeconds: int32(auth.PersistentTTL / time.Second),
		TokenHash:  row.TokenHash,
	})
}

// StartSessionSweeper deletes expired login rows on a ticker until ctx is done.
//
// Nothing depends on it for correctness — GetUserSession filters on expires_at,
// so an expired row is already dead — it exists so the table and the rate
// limiter's map do not grow without bound over the life of the deployment.
func (s *Server) StartSessionSweeper(ctx context.Context, every time.Duration) {
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := s.q.DeleteExpiredUserSessions(ctx); err != nil {
					log.Printf("session sweep: %v", err)
				}
				s.logins.Sweep()
			}
		}
	}()
}

// secureCookies reports whether session cookies should carry the Secure flag.
// Local development is plain HTTP, where a Secure cookie is never sent back and
// logins would appear to succeed but never stick.
func (s *Server) secureCookies() bool {
	return s.environment != "development"
}

// sameOrigin rejects state-changing requests that announce a foreign origin.
//
// SameSite=Lax on the session cookie is the primary CSRF defence and is
// sufficient on its own for a same-origin SPA; this is a second, independent
// check so that a browser bug or a future SameSite=None does not silently
// remove the only one. Requests with no Origin header pass: non-browser clients
// (curl, the integration suite, probes) do not send one, and they are not the
// threat CSRF describes.
//
// The UI is same-origin with the API in production (Traefik path-routes /api
// and preserves Host) and in development (the Vite proxy, which must therefore
// leave changeOrigin off — see src/ui/vite.config.ts).
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin != "" && !originMatchesHost(origin, r.Host) {
			writeError(w, http.StatusForbidden, "cross_origin", "cross-origin request rejected")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func originMatchesHost(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == host
}
