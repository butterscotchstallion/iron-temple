package api_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
	"github.com/jackc/pgx/v5/pgconn"

	"gitea.homelab/gitadmin/iron-temple/api/internal/auth"
)

// ---- registration ----

func TestRegistrationClosesAfterTheFirstAccount(t *testing.T) {
	e := expectAnon(t)

	// TestMain already claimed the install.
	e.GET("/auth/registration-status").Expect().
		Status(http.StatusOK).
		JSON().Object().HasValue("open", false)

	e.POST("/auth/register").
		WithJSON(map[string]any{"username": "interloper", "password": "another-password"}).
		Expect().Status(http.StatusForbidden).
		JSON().Object().HasValue("code", "registration_closed")
}

// The first-user guard cannot rest on the application's count check alone.
// "Is the table empty?" asks about rows that do not exist, and COUNT takes no
// predicate lock on them, so two concurrent registrations with different
// usernames could both see an empty table and both commit. register() holds an
// advisory lock to serialize that, and users_single_admin_idx is the backstop
// underneath it — this asserts the backstop directly, by going around the API
// and trying to write a second owner.
func TestDatabaseRefusesASecondAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a Docker daemon")
	}

	_, err := testPool.Exec(context.Background(),
		`INSERT INTO users (username, display_name, password_hash, is_admin)
		 VALUES ('second-owner', 'Second Owner', 'x', true)`)
	if err == nil {
		// Undo it, or every later test runs against a two-owner install.
		_, _ = testPool.Exec(context.Background(),
			`DELETE FROM users WHERE username = 'second-owner'`)
		t.Fatal("the database accepted a second admin — users_single_admin_idx is missing or not partial")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("insert failed, but not with a unique violation: %v", err)
	}
	if pgErr.ConstraintName != "users_single_admin_idx" {
		t.Errorf("rejected by %q, want users_single_admin_idx", pgErr.ConstraintName)
	}

	// A non-admin account is still allowed: the index is partial, so it
	// constrains owners without freezing the table.
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO users (username, display_name, password_hash, is_admin)
		 VALUES ('ordinary', 'Ordinary', 'x', false)`); err != nil {
		t.Fatalf("the index also blocks ordinary accounts: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			`DELETE FROM users WHERE username = 'ordinary'`)
	})
}

// Concurrent registrations must produce exactly one account. Registration is
// already closed by TestMain, so every one of these must be refused — and
// refused cleanly, with a 403, rather than surfacing a constraint violation as
// a 500.
func TestConcurrentRegistrationsAreAllRefusedCleanly(t *testing.T) {
	e := expectAnon(t)

	const attempts = 8
	var wg sync.WaitGroup
	codes := make([]int, attempts)
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := e.POST("/auth/register").
				WithJSON(map[string]any{
					"username": fmt.Sprintf("racer%d", i),
					"password": "a-long-enough-password",
				}).
				Expect()
			codes[i] = resp.Raw().StatusCode
		}()
	}
	wg.Wait()

	for i, code := range codes {
		if code != http.StatusForbidden {
			t.Errorf("attempt %d returned %d, want 403", i, code)
		}
	}
}

func TestRegisterValidatesCredentials(t *testing.T) {
	e := expectAnon(t)

	// Validation runs before the registration-closed check, so these are 400s
	// rather than 403s even though the install is claimed.
	tests := map[string]map[string]any{
		"username too short": {"username": "ab", "password": "long-enough-pw"},
		"username too long":  {"username": string(make([]byte, 0, 40)) + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "password": "long-enough-pw"},
		"username has space": {"username": "not ok", "password": "long-enough-pw"},
		"username has slash": {"username": "a/b", "password": "long-enough-pw"},
		"password too short": {"username": "someone", "password": "short"},
		"password missing":   {"username": "someone"},
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			e.POST("/auth/register").WithJSON(body).
				Expect().Status(http.StatusBadRequest)
		})
	}
}

// ---- login and logout ----

func TestLoginRejectsWrongPassword(t *testing.T) {
	e := expectAnon(t)
	e.POST("/auth/login").
		WithJSON(map[string]any{"username": primaryUsername, "password": "not-the-password"}).
		Expect().Status(http.StatusUnauthorized)
}

// An unknown username and a wrong password must be indistinguishable, or the
// endpoint becomes an account-enumeration oracle.
func TestLoginGivesTheSameAnswerForUnknownUsers(t *testing.T) {
	e := expectAnon(t)

	unknown := e.POST("/auth/login").
		WithJSON(map[string]any{"username": "nobody-here", "password": "not-the-password"}).
		Expect().Status(http.StatusUnauthorized).JSON().Object()
	known := e.POST("/auth/login").
		WithJSON(map[string]any{"username": primaryUsername, "password": "not-the-password"}).
		Expect().Status(http.StatusUnauthorized).JSON().Object()

	unknown.HasValue("code", known.Value("code").String().Raw())
	unknown.HasValue("message", known.Value("message").String().Raw())
}

func TestLoginIsCaseInsensitiveOnUsername(t *testing.T) {
	e := expectAnon(t)
	e.POST("/auth/login").
		WithJSON(map[string]any{"username": "PRIMARY", "password": primaryPassword}).
		Expect().Status(http.StatusOK).
		JSON().Object().HasValue("username", primaryUsername)
}

// "Remember me" is the promise that you stay signed in. Without it the cookie
// must die with the browser session; with it, it must outlive it.
func TestRememberMeControlsCookieLifetime(t *testing.T) {
	tests := []struct {
		name       string
		rememberMe bool
		wantMaxAge time.Duration // 0 means the attribute must be absent
	}{
		{"without remember me it is a browser-session cookie", false, 0},
		{"with remember me it lasts 60 days", true, 60 * 24 * time.Hour},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := expectAnon(t)
			resp := e.POST("/auth/login").
				WithJSON(map[string]any{
					"username": primaryUsername, "password": primaryPassword,
					"rememberMe": tc.rememberMe,
				}).
				Expect().Status(http.StatusOK)

			c := resp.Cookie(sessionCookie)
			c.Value().NotEmpty()
			if tc.wantMaxAge == 0 {
				// No Max-Age and no Expires: the browser drops it on close.
				c.NotContainsMaxAge()
			} else {
				c.ContainsMaxAge()
				c.MaxAge().IsEqual(tc.wantMaxAge)
			}
		})
	}
}

func TestSessionCookieIsHardened(t *testing.T) {
	e := expectAnon(t)
	raw := e.POST("/auth/login").
		WithJSON(map[string]any{"username": primaryUsername, "password": primaryPassword}).
		Expect().Status(http.StatusOK).Raw()
	defer func() { _ = raw.Body.Close() }()

	var got *http.Cookie
	for _, c := range raw.Cookies() {
		if c.Name == sessionCookie {
			got = c
		}
	}
	if got == nil {
		t.Fatalf("no %s cookie was set", sessionCookie)
	}
	if !got.HttpOnly {
		t.Error("session cookie is not HttpOnly — script could read it")
	}
	if got.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax (the CSRF defence)", got.SameSite)
	}
	// The test server runs with environment "" (not "development"), which is
	// how production is configured.
	if !got.Secure {
		t.Error("session cookie is not Secure outside development")
	}
}

func TestProtectedEndpointsRequireASession(t *testing.T) {
	e := expectAnon(t)

	for _, path := range []string{"/me", "/sessions", "/exercises", "/programs"} {
		t.Run(path, func(t *testing.T) {
			e.GET(path).Expect().Status(http.StatusUnauthorized).
				JSON().Object().HasValue("code", "unauthenticated")
		})
	}
}

func TestUnknownSessionCookieIsRejectedAndCleared(t *testing.T) {
	e := expectAs(t, "this-token-was-never-issued")

	resp := e.GET("/me").Expect().Status(http.StatusUnauthorized)
	// The stale cookie is expired in the response, or the UI would 401 forever
	// with no way to recover short of clearing site data by hand. httpexpect
	// reports a negative Max-Age (delete now) as a zero duration.
	c := resp.Cookie(sessionCookie)
	c.ContainsMaxAge()
	c.MaxAge().IsEqual(0)
}

// Logout has to revoke server-side. A cookie the client drops but the server
// still honours is misplaced, not revoked.
func TestLogoutRevokesTheSessionServerSide(t *testing.T) {
	token := login(t, primaryUsername, primaryPassword)
	e := expectAs(t, token)

	e.GET("/me").Expect().Status(http.StatusOK)
	e.POST("/auth/logout").Expect().Status(http.StatusNoContent)
	e.GET("/me").Expect().Status(http.StatusUnauthorized)
}

// ---- per-user isolation ----

// The isolation model is one WHERE clause per query, and a missed one fails
// nothing at compile time — it just serves someone else's data. This is the
// test that would catch it.
func TestOneUserCannotReachAnothersSessions(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	// A session belonging to the primary user.
	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	setID := int(created.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())

	other := expectAs(t, secondUserToken(t))

	// Every route that takes a session or set id must answer 404 — not 403,
	// which would confirm the id exists.
	other.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusNotFound)
	other.PATCH(fmt.Sprintf("/sessions/%d", sessionID)).
		WithJSON(map[string]any{"notes": "not mine"}).
		Expect().Status(http.StatusNotFound)
	other.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusNotFound)
	other.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, setID)).
		WithJSON(map[string]any{"actualReps": 5}).
		Expect().Status(http.StatusNotFound)
	other.DELETE(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusNotFound)

	// And the session must survive all of that.
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusOK)
}

func TestSessionListsAreScopedToTheirOwner(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	// Log a rep so the session counts as started and appears in listings.
	setID := int(created.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, setID)).
		WithJSON(map[string]any{"actualReps": 5, "completed": true}).
		Expect().Status(http.StatusOK)

	other := expectAs(t, secondUserToken(t))
	other.GET("/sessions").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("total", 0)
	other.GET("/sessions").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array().IsEmpty()
}

// The progression engine reads lift history. Unscoped, it would put one
// lifter's next working weight in front of another — a wrong number on the bar,
// not merely a privacy leak.
func TestProgressionDoesNotSeeAnotherUsersHistory(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)

	other := expectAs(t, secondUserToken(t))
	_, otherWeightBefore := firstExercisePreview(other, programID, dayID)

	// The primary user completes a full session, which advances their own
	// prescription.
	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	sets := created.Value("sets").Array()
	firstExerciseID := sets.Value(0).Object().Value("exerciseId").Number().Raw()
	for i := 0; i < int(sets.Length().Raw()); i++ {
		set := sets.Value(i).Object()
		if set.Value("exerciseId").Number().Raw() != firstExerciseID {
			continue
		}
		e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", sessionID,
			int(set.Value("id").Number().Raw()))).
			WithJSON(map[string]any{
				"actualReps": int(set.Value("targetReps").Number().Raw()),
				"completed":  true,
			}).
			Expect().Status(http.StatusOK)
	}
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)

	_, mineAfter := firstExercisePreview(e, programID, dayID)
	if mineAfter <= otherWeightBefore {
		t.Fatalf("precondition: the primary user's weight should have advanced (%v -> %v)",
			otherWeightBefore, mineAfter)
	}

	_, otherWeightAfter := firstExercisePreview(other, programID, dayID)
	if otherWeightAfter != otherWeightBefore {
		t.Errorf("another user's completed session moved this user's prescription: %v -> %v",
			otherWeightBefore, otherWeightAfter)
	}

	// Exercise history is scoped by the same rule.
	exerciseID := int(firstExerciseID)
	e.GET(fmt.Sprintf("/exercises/%d/history", exerciseID)).
		Expect().Status(http.StatusOK).JSON().Array().NotEmpty()
	other.GET(fmt.Sprintf("/exercises/%d/history", exerciseID)).
		Expect().Status(http.StatusOK).JSON().Array().IsEmpty()
}

// ---- profile ----

func TestUpdateProfile(t *testing.T) {
	e := expect(t)
	t.Cleanup(func() {
		e.PATCH("/me").
			WithJSON(map[string]any{"displayName": "Primary Lifter", "avatarColor": ""}).
			Expect().Status(http.StatusOK)
	})

	updated := e.PATCH("/me").
		WithJSON(map[string]any{"displayName": "Ada", "avatarColor": "#b026ff"}).
		Expect().Status(http.StatusOK).JSON().Object()
	updated.HasValue("displayName", "Ada")
	updated.HasValue("avatarColor", "#b026ff")

	e.GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("displayName", "Ada")
}

func TestUpdateProfileRejectsBadInput(t *testing.T) {
	e := expect(t)

	// The colour lands in a style attribute, so anything that isn't a hex
	// colour is refused rather than escaped.
	for _, colour := range []string{"red", "#xyzxyz", "javascript:alert(1)", "#b026ff; x"} {
		e.PATCH("/me").WithJSON(map[string]any{"avatarColor": colour}).
			Expect().Status(http.StatusBadRequest)
	}
	e.PATCH("/me").WithJSON(map[string]any{"displayName": "   "}).
		Expect().Status(http.StatusBadRequest)
}

// The current password is required even though the caller is already signed in:
// it is what stops a borrowed session becoming permanent ownership.
func TestChangePasswordRequiresTheCurrentOne(t *testing.T) {
	e := expect(t)
	e.PUT("/me/password").
		WithJSON(map[string]any{"currentPassword": "wrong", "newPassword": "a-brand-new-password"}).
		Expect().Status(http.StatusUnauthorized)
}

// Runs against its own account, deliberately.
//
// Changing a password revokes every *other* session for that user — that is the
// feature. Pointed at the primary account it also revokes primaryToken, the
// session TestMain mints and expect() hands to every other test in the package,
// so each test that ran afterwards failed with a 401 that had nothing to do
// with what it was testing. A dedicated user keeps the blast radius inside the
// test that causes it.
func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	const (
		username    = "rotator"
		oldPassword = "the-original-password"
		newPassword = "a-brand-new-password"
	)
	createUser(t, username, "Password Rotator", oldPassword)

	// Two independent logins for that user.
	keep := expectAs(t, login(t, username, oldPassword))
	revoked := expectAs(t, login(t, username, oldPassword))
	revoked.GET("/me").Expect().Status(http.StatusOK)

	keep.PUT("/me/password").
		WithJSON(map[string]any{
			"currentPassword": oldPassword, "newPassword": newPassword,
		}).
		Expect().Status(http.StatusNoContent)
	// Restore it, so a re-run (go test -count=2) finds the password createUser
	// expects rather than an account it can no longer log into.
	t.Cleanup(func() {
		keep.PUT("/me/password").
			WithJSON(map[string]any{
				"currentPassword": newPassword, "newPassword": oldPassword,
			}).
			Expect().Status(http.StatusNoContent)
	})

	// The session that made the change survives, so the user isn't signed out
	// of the tab they just used...
	keep.GET("/me").Expect().Status(http.StatusOK)
	// ...but every other one is revoked, which is the entire point of changing
	// a password you believe was compromised.
	revoked.GET("/me").Expect().Status(http.StatusUnauthorized)

	// The new password works and the old one does not.
	expectAnon(t).POST("/auth/login").
		WithJSON(map[string]any{"username": username, "password": oldPassword}).
		Expect().Status(http.StatusUnauthorized)
	expectAnon(t).POST("/auth/login").
		WithJSON(map[string]any{"username": username, "password": newPassword}).
		Expect().Status(http.StatusOK)

	// The primary session is untouched. Without this, an edit that points the
	// test back at the primary account would break fifteen unrelated tests and
	// give no clue why — which is what happened the first time this ran.
	expect(t).GET("/me").Expect().Status(http.StatusOK)
}

// ---- avatars ----

func TestAvatarUploadServeAndDelete(t *testing.T) {
	e := expect(t)
	t.Cleanup(func() { e.DELETE("/me/avatar").Expect().Status(http.StatusNoContent) })

	e.GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("hasAvatar", false)

	etag := e.POST("/me/avatar").
		WithMultipart().
		WithFileBytes("avatar", "avatar.png", pngBytes(t, 64, 64)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("etag").String().NotEmpty().Raw()

	me := e.GET("/me").Expect().Status(http.StatusOK).JSON().Object()
	me.HasValue("hasAvatar", true)
	me.HasValue("avatarEtag", etag)

	userID := int(me.Value("id").Number().Raw())
	img := expectAnon(t).GET(fmt.Sprintf("/users/%d/avatar", userID)).
		Expect().Status(http.StatusOK)
	img.HasContentType("image/png")
	img.Header("ETag").NotEmpty()

	// A conditional re-fetch is cheap, which is what makes must-revalidate a
	// reasonable caching policy here.
	expectAnon(t).GET(fmt.Sprintf("/users/%d/avatar", userID)).
		WithHeader("If-None-Match", img.Header("ETag").Raw()).
		Expect().Status(http.StatusNotModified)

	e.DELETE("/me/avatar").Expect().Status(http.StatusNoContent)
	e.GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("hasAvatar", false)
	expectAnon(t).GET(fmt.Sprintf("/users/%d/avatar", userID)).
		Expect().Status(http.StatusNotFound)
}

// A Content-Type header is client-supplied and proves nothing; decoding the
// bytes is the actual check.
func TestAvatarRejectsNonImages(t *testing.T) {
	e := expect(t)
	e.POST("/me/avatar").
		WithMultipart().
		WithFileBytes("avatar", "avatar.png", []byte("this is not a PNG, whatever the name says")).
		Expect().Status(http.StatusBadRequest)
}

func TestAvatarRejectsOversizedUploads(t *testing.T) {
	e := expect(t)
	// Comfortably past the 256 KB cap. Random-ish pixel data so PNG's
	// compression cannot shrink it back under the limit.
	e.POST("/me/avatar").
		WithMultipart().
		WithFileBytes("avatar", "avatar.png", noisyPNG(t, 512, 512)).
		Expect().Status(http.StatusRequestEntityTooLarge)
}

func TestAvatarDeleteIsIdempotent(t *testing.T) {
	// Removing an avatar that was never uploaded reaches the caller's desired
	// state, so it is a success rather than a 404.
	e := expect(t)
	e.DELETE("/me/avatar").Expect().Status(http.StatusNoContent)
	e.DELETE("/me/avatar").Expect().Status(http.StatusNoContent)
}

func TestUnknownUserAvatarIsNotFound(t *testing.T) {
	expectAnon(t).GET("/users/999999/avatar").Expect().Status(http.StatusNotFound)
}

// ---- CSRF ----

func TestCrossOriginMutationsAreRejected(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	e.POST("/sessions").
		WithHeader("Origin", "https://evil.example").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusForbidden).
		JSON().Object().HasValue("code", "cross_origin")

	// Reads are untouched — the check is about state change.
	e.GET("/sessions").
		WithHeader("Origin", "https://evil.example").
		Expect().Status(http.StatusOK)
}

// ---- helpers ----

// login signs in and returns the session token.
func login(t *testing.T, username, password string) string {
	t.Helper()
	resp := expectAnon(t).POST("/auth/login").
		WithJSON(map[string]any{"username": username, "password": password}).
		Expect().Status(http.StatusOK)
	return resp.Cookie(sessionCookie).Value().NotEmpty().Raw()
}

// secondUserToken returns a session for a second account, creating it on first
// use. Registration closed itself after the primary user, so the row is written
// directly — the same way backdateSession reaches past the API for setup the
// API deliberately does not expose.
func secondUserToken(t *testing.T) string {
	t.Helper()
	return createUser(t, "secondary", "Secondary Lifter", "second-user-password")
}

// createUser inserts an account directly and returns a session token for it.
// Registration is first-user-only and TestMain already claimed the install, so
// extra accounts are written straight to the database — the same reaching past
// the API that backdateSession does. Idempotent, so callers need not care
// whether an earlier test already made it.
func createUser(t *testing.T, username, displayName, password string) string {
	t.Helper()
	// Skip before touching testPool: under -short, TestMain returns without
	// booting a database and the pool is nil. expectAnon does the same, but a
	// caller may reach this helper first.
	if testing.Short() {
		t.Skip("integration test requires a Docker daemon")
	}

	hash, err := auth.PBKDF2Hasher{}.Hash(password)
	if err != nil {
		t.Fatalf("hash password for %s: %v", username, err)
	}
	_, err = testPool.Exec(context.Background(),
		`INSERT INTO users (username, display_name, password_hash)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (username) DO NOTHING`,
		username, displayName, hash)
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return login(t, username, password)
}

// pngBytes builds a small valid PNG.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x80, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// noisyPNG builds a PNG that does not compress well, so its encoded size
// actually exceeds the upload cap.
func noisyPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// A deterministic but high-entropy pattern: no PRNG, so the test is
	// reproducible, but adjacent pixels don't correlate.
	seed := uint32(0x9e3779b9)
	for y := range h {
		for x := range w {
			seed = seed*1664525 + 1013904223
			img.Set(x, y, color.RGBA{
				R: uint8(seed >> 24), G: uint8(seed >> 16), B: uint8(seed >> 8), A: 0xff,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if buf.Len() <= 256<<10 {
		t.Fatalf("test fixture is only %d bytes, not over the 256 KB cap", buf.Len())
	}
	return buf.Bytes()
}

// compile-time check that the helper signature matches httpexpect's.
var _ = func(e *httpexpect.Expect) *httpexpect.Request { return e.GET("/") }
