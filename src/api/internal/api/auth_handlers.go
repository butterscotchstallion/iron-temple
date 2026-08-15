package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"gitea.homelab/gitadmin/iron-temple/api/internal/auth"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// pgerrcodeUniqueViolation is SQLSTATE 23505. Spelled out rather than pulled
// from a constants package so the module keeps its current dependency set.
const pgerrcodeUniqueViolation = "23505"

// Credential limits.
const (
	minUsernameLen = 3
	maxUsernameLen = 32
	minPasswordLen = 8
	// maxPasswordLen bounds the work one request can ask for. PBKDF2's cost is
	// dominated by its iteration count, not the password, but HMAC still walks
	// the input once per iteration — so an unbounded password is a way to buy
	// far more CPU than a login should cost.
	maxPasswordLen  = 256
	maxDisplayName  = 64
	maxAvatarColour = 32
)

// usernamePattern keeps usernames to characters that are unambiguous in a URL,
// a log line, and a shell — the places a username tends to end up.
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type registerRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
	RememberMe  bool   `json:"rememberMe"`
}

type loginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RememberMe bool   `json:"rememberMe"`
}

// getRegistrationStatus tells the UI whether to offer "create account" or only
// "sign in". Deliberately unauthenticated — it is the one thing a signed-out
// visitor must know to render the right form — and deliberately a bare boolean:
// the user count is nobody's business.
func (s *Server) getRegistrationStatus(w http.ResponseWriter, r *http.Request) {
	open, err := s.registrationOpen(r.Context())
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, registrationStatusDTO{Open: open})
}

func (s *Server) registrationOpen(ctx context.Context) (bool, error) {
	n, err := s.q.CountUsers(ctx)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// registrationLockKey is the advisory-lock key register() serializes on. The
// value only has to be stable and not collide with another advisory lock in
// this database, and this is the only one the app takes. ASCII "IRON", so it is
// recognisable in pg_locks when someone is wondering what holds it.
const registrationLockKey int64 = 0x49524F4E

// register creates the first account and signs it in.
//
// Registration closes as soon as one account exists. This is a homelab install
// reachable from the internet: an open signup form is an open door, and an
// invite-code scheme would be one more secret to manage for an app that expects
// exactly one lifter.
//
// Closing it properly takes more than a transaction. "Is the table empty?" is a
// question about rows that do not exist, and a COUNT over them takes no lock a
// second transaction would block on — so under READ COMMITTED two racing
// registrations with different usernames would both see an empty table and both
// commit, leaving two owners. A transaction makes the check and the insert
// atomic, not mutually exclusive. The advisory lock below is what actually
// serializes them; users_single_admin_idx is the database-level backstop.
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	username := strings.TrimSpace(req.Username)
	if msg, ok := validateUsername(username); !ok {
		badRequest(w, msg)
		return
	}
	if msg, ok := validatePassword(req.Password); !ok {
		badRequest(w, msg)
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	if utf8.RuneCountInString(displayName) > maxDisplayName {
		badRequest(w, "displayName must be at most 64 characters")
		return
	}

	hash, err := s.hasher.Hash(req.Password)
	if err != nil {
		internalError(w)
		return
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	// Before the check, not after: this is what makes the check meaningful.
	// A concurrent registration blocks here until this transaction ends, then
	// sees the row this one wrote and is refused.
	if err := qtx.LockRegistration(ctx, registrationLockKey); err != nil {
		internalError(w)
		return
	}

	n, err := qtx.CountUsers(ctx)
	if err != nil {
		internalError(w)
		return
	}
	if n > 0 {
		forbidden(w, "registration_closed", "registration is closed")
		return
	}

	user, err := qtx.CreateUser(ctx, store.CreateUserParams{
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: hash,
		// The founder administers the install; there is nobody else to do it.
		IsAdmin: true,
	})
	if err != nil {
		// users_single_admin_idx rejecting a second owner, or the username
		// index rejecting a duplicate. Either way somebody got here first, and
		// the honest answer is the same one the count check gives. Unreachable
		// while the advisory lock above is held — this is the backstop
		// answering rather than a 500.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcodeUniqueViolation {
			forbidden(w, "registration_closed", "registration is closed")
			return
		}
		internalError(w)
		return
	}

	// Sessions logged before accounts existed have no owner and are invisible
	// to every scoped query. Hand them to the first account, or this install's
	// entire training history vanishes the moment someone signs up.
	if _, err := qtx.AdoptOrphanSessions(ctx, user.ID); err != nil {
		internalError(w)
		return
	}

	token, err := s.issueSession(ctx, qtx, user.ID, req.RememberMe)
	if err != nil {
		internalError(w)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	auth.SetCookie(w, token, req.RememberMe, s.secureCookies())
	writeJSON(w, http.StatusCreated, userDTO{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		AvatarColor: user.AvatarColor,
		IsAdmin:     user.IsAdmin,
		HasAvatar:   false,
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		badRequest(w, "username and password are required")
		return
	}
	if len(req.Password) > maxPasswordLen {
		// Refuse before hashing — see maxPasswordLen.
		unauthorized(w, "invalid username or password")
		return
	}

	// Rate-limit on client address *and* username, so one attacker cannot lock
	// out an account by guessing at it, and cannot dodge the limit by rotating
	// usernames from the same host either.
	key := clientIP(r) + "|" + strings.ToLower(username)
	if !s.logins.Allow(key) {
		writeError(w, http.StatusTooManyRequests, "too_many_attempts",
			"too many failed sign-in attempts; try again later")
		return
	}

	user, err := s.q.GetUserForLogin(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		// Spend the same CPU as a real verification. Returning immediately here
		// would make an unknown username measurably faster than a known one,
		// which is an account-enumeration oracle.
		s.hasher.DummyVerify(req.Password)
		s.logins.Fail(key)
		unauthorized(w, "invalid username or password")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	ok, needsRehash := s.hasher.Verify(req.Password, user.PasswordHash)
	if !ok {
		s.logins.Fail(key)
		unauthorized(w, "invalid username or password")
		return
	}
	s.logins.Reset(key)

	// The password is in hand and already verified, so this is the one moment a
	// stored hash can be upgraded to current parameters. Best-effort: failing to
	// re-hash is not a reason to fail the login.
	if needsRehash {
		if fresh, err := s.hasher.Hash(req.Password); err == nil {
			_, _ = s.q.UpdateUserPassword(ctx, store.UpdateUserPasswordParams{
				PasswordHash: fresh, ID: user.ID,
			})
		}
	}

	token, err := s.issueSession(ctx, s.q, user.ID, req.RememberMe)
	if err != nil {
		internalError(w)
		return
	}

	auth.SetCookie(w, token, req.RememberMe, s.secureCookies())
	writeJSON(w, http.StatusOK, s.userDTO(ctx, store.GetUserRow{
		ID:               user.ID,
		Username:         user.Username,
		DisplayName:      user.DisplayName,
		AvatarColor:      user.AvatarColor,
		IsAdmin:          user.IsAdmin,
		CurrentProgramID: user.CurrentProgramID,
	}))
}

// logout revokes the presented session server-side and clears the cookie.
// Deleting the row is what makes this a real logout: a cookie the client
// discards but the server still honours is not revoked, merely misplaced.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if err := s.q.DeleteUserSession(r.Context(), u.tokenHash); err != nil {
		internalError(w)
		return
	}
	auth.ClearCookie(w, s.secureCookies())
	w.WriteHeader(http.StatusNoContent)
}

// issueSession mints a token and records it. q is the caller's Queries — the
// transaction's during registration, the pool's during login.
func (s *Server) issueSession(ctx context.Context, q *store.Queries, userID int32, persistent bool) (string, error) {
	token, digest, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	err = q.CreateUserSession(ctx, store.CreateUserSessionParams{
		TokenHash:  digest,
		UserID:     userID,
		ExpiresAt:  pgtype.Timestamptz{Time: time.Now().Add(auth.TTL(persistent)), Valid: true},
		Persistent: persistent,
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func validateUsername(username string) (string, bool) {
	n := utf8.RuneCountInString(username)
	if n < minUsernameLen || n > maxUsernameLen {
		return "username must be between 3 and 32 characters", false
	}
	if !usernamePattern.MatchString(username) {
		return "username may contain only letters, digits, dot, underscore and hyphen", false
	}
	return "", true
}

func validatePassword(password string) (string, bool) {
	// Counted in runes, not bytes, so a passphrase of eight non-ASCII
	// characters is not rejected for being "too short".
	if utf8.RuneCountInString(password) < minPasswordLen {
		return "password must be at least 8 characters", false
	}
	if len(password) > maxPasswordLen {
		return "password must be at most 256 bytes", false
	}
	return "", true
}

// clientIP is the rate-limiter key's host part. It reads the socket address
// only — never X-Forwarded-For, which the client controls and could vary per
// request to get an unlimited number of buckets. In this deployment Traefik is
// the only thing that dials the API, so the socket address is the honest answer
// available; behind a different proxy this would need a trusted-hop config.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
