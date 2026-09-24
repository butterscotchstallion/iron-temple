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
	// MustChangePassword is set on accounts the admin area created, whose first
	// password was chosen by somebody else. It rides the session join for the
	// same reason IsAdmin does — blockUntilPasswordChanged consults it on every
	// authenticated request, and a second query per request to answer "may this
	// one proceed" is a poor trade against one more column on a join already
	// being made.
	MustChangePassword bool
	// CurrentProgramID is the program the user last opened, nil until they open
	// one. It rides the session join rather than a second query because getMe
	// serves the whole profile from this struct.
	CurrentProgramID *int32
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
		ID:                 row.UserID,
		Username:           row.Username,
		DisplayName:        row.DisplayName,
		AvatarColor:        row.AvatarColor,
		IsAdmin:            row.IsAdmin,
		MustChangePassword: row.MustChangePassword,
		CurrentProgramID:   row.CurrentProgramID,
		tokenHash:          digest,
	}, true
}

// requireAdmin rejects a caller who does not administer this install.
//
// Mounts under requireUser and nowhere else: it reads the authenticated user
// through userFrom, which panics rather than guesses when there isn't one. That
// is the behaviour wanted — an admin route accidentally mounted outside the
// session middleware must fail loudly in tests, not quietly admit everybody.
//
// There is exactly one admin per install (users_single_admin_idx), so this is
// "is this the owner?" rather than the first rung of a role hierarchy. If more
// roles ever arrive, this is the seam they arrive at.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !userFrom(r.Context()).IsAdmin {
			// 403, not 404. Hiding the route's existence would buy nothing here
			// — it is documented in the spec the UI is generated from — and a
			// 404 would send an admin whose session had silently become an
			// ordinary one hunting for a broken link instead of a lost session.
			forbidden(w, "admin_required", "this account does not administer this install")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// blockUntilPasswordChanged refuses an account that is still carrying the
// one-time password the admin gave it.
//
// The exemptions are not listed here, they are expressed by where this mounts:
// GET /me and PUT /me/password sit outside it in Router, and everything else
// sits inside. That is deliberate. Matching on the route pattern would not work
// — chi populates it during routing, which happens after middleware runs (see
// the comment on observe) — and matching on the raw path would put the
// allowlist somewhere a new route could quietly disagree with it. Mounting is
// the check, so the gate is on by default and an exemption has to be written on
// purpose.
//
// 403 rather than 401: the session is perfectly valid and the credentials are
// not in question. A 401 would tell the UI to show the sign-in form, which is
// the one screen that cannot help — signing in again lands in exactly the same
// state.
func (s *Server) blockUntilPasswordChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userFrom(r.Context()).MustChangePassword {
			forbidden(w, "password_change_required",
				"set a new password before using this account")
			return
		}
		next.ServeHTTP(w, r)
	})
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
// limiters' maps do not grow without bound over the life of the deployment.
//
// Both limiters are swept here rather than each owning a ticker. They have the
// same problem (a map keyed by something a caller chooses) and the same
// answer, and one loop doing two cheap map walks is not worth a second.
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
				s.comments.Sweep()
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
