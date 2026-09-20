package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// The admin area and the forced password change it hands out.
//
// The suite's primary account registered first, so it owns the install and is
// the admin; every non-admin session below is minted by creating an account
// through the endpoint under test and signing in as it. That is not a shortcut
// — it is the only way a second account can exist at all, which is the point of
// the feature.

// createAccount adds an ordinary account as the admin and returns the created
// row. Registers a cleanup that removes it, because every test in this package
// shares one database and a roster that grows with the suite is a roster no
// test can make an exact assertion about.
func createAccount(t *testing.T, username, password string) *httpexpect.Object {
	t.Helper()
	created := expect(t).POST("/admin/users").
		WithJSON(map[string]any{"username": username, "password": password}).
		Expect().Status(http.StatusCreated).
		JSON().Object()

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			`DELETE FROM users WHERE lower(username) = lower($1)`, username)
	})
	return created
}

// signIn returns a session token for an existing account.
func signIn(t *testing.T, username, password string) string {
	t.Helper()
	return expectAnon(t).POST("/auth/login").
		WithJSON(map[string]any{"username": username, "password": password}).
		Expect().Status(http.StatusOK).
		Cookie(sessionCookie).Value().Raw()
}

// ---- the gate ----

func TestAdminRoutesRejectAnonymousCallers(t *testing.T) {
	e := expectAnon(t)
	e.GET("/admin/users").Expect().Status(http.StatusUnauthorized)
	e.POST("/admin/users").
		WithJSON(map[string]any{"username": "nobody", "password": "long-enough-pw"}).
		Expect().Status(http.StatusUnauthorized)
}

// An ordinary account must not be able to read the roster or mint more
// accounts. This is the whole of the authorisation model, so it is asserted on
// both verbs rather than on the one that happens to be cheaper to call.
func TestAdminRoutesRejectNonAdmins(t *testing.T) {
	createAccount(t, "ordinary-caller", "ordinary-caller-pw")
	token := signIn(t, "ordinary-caller", "ordinary-caller-pw")
	// The account still owes a password change, which would 403 on its own and
	// hide whether requireAdmin works at all. Clear it first, so what is being
	// asserted below is the admin rule and nothing else.
	expectAs(t, token).PUT("/me/password").
		WithJSON(map[string]any{
			"currentPassword": "ordinary-caller-pw",
			"newPassword":     "ordinary-caller-new-pw",
		}).
		Expect().Status(http.StatusNoContent)

	e := expectAs(t, token)
	e.GET("/admin/users").Expect().
		Status(http.StatusForbidden).
		JSON().Object().HasValue("code", "admin_required")
	e.POST("/admin/users").
		WithJSON(map[string]any{"username": "nested", "password": "long-enough-pw"}).
		Expect().
		Status(http.StatusForbidden).
		JSON().Object().HasValue("code", "admin_required")
}

// ---- listing ----

func TestAdminListsEveryAccount(t *testing.T) {
	created := createAccount(t, "roster-member", "roster-member-pw")

	users := expect(t).GET("/admin/users").Expect().
		Status(http.StatusOK).
		JSON().Array()

	var sawOwner, sawMember bool
	for _, u := range users.Iter() {
		row := u.Object()
		// No endpoint may ever put a hash on the wire. Asserted here rather
		// than trusted to the DTO having no field for one, because this is the
		// one response that serializes accounts the caller does not own.
		row.NotContainsKey("password")
		row.NotContainsKey("passwordHash")

		switch row.Value("username").String().Raw() {
		case primaryUsername:
			sawOwner = true
			row.HasValue("isAdmin", true)
			// The owner chose their own password at registration, so they were
			// never handed a one-time one.
			row.HasValue("mustChangePassword", false)
		case "roster-member":
			sawMember = true
			row.HasValue("isAdmin", false)
			row.HasValue("mustChangePassword", true)
			row.Value("createdAt").String().NotEmpty()
		}
	}
	if !sawOwner {
		t.Error("the roster omits the account that claimed the install")
	}
	if !sawMember {
		t.Errorf("the roster omits the account just created (id %v)",
			created.Value("id").Raw())
	}
}

// ---- creating ----

func TestAdminCreatesAnOrdinaryAccount(t *testing.T) {
	created := createAccount(t, "grace", "grace-first-password")

	created.Value("username").String().IsEqual("grace")
	// Omitted from the request, so it falls back to the username exactly as
	// registration does.
	created.Value("displayName").String().IsEqual("grace")
	// Not negotiable by the caller: the install has one admin and it is not
	// this account.
	created.HasValue("isAdmin", false)
	// The password came from somebody who will never use the account.
	created.HasValue("mustChangePassword", true)
	created.Value("createdAt").String().NotEmpty()
	created.NotContainsKey("password")
}

func TestAdminCreateHonoursAGivenDisplayName(t *testing.T) {
	expect(t).POST("/admin/users").
		WithJSON(map[string]any{
			"username":    "hopper",
			"displayName": "Grace Hopper",
			"password":    "hopper-first-password",
		}).
		Expect().Status(http.StatusCreated).
		JSON().Object().HasValue("displayName", "Grace Hopper")

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			`DELETE FROM users WHERE username = 'hopper'`)
	})
}

// The account must arrive with the standard plate set, in the same transaction
// that created it. Without this, an admin-created lifter gets bar-only plate
// maths and a silently different app from the owner's — see 0013_gym_setup on
// why an empty inventory has to mean "owns no plates" rather than "never
// configured".
func TestAdminCreatedAccountOwnsTheDefaultPlates(t *testing.T) {
	createAccount(t, "plated", "plated-first-password")
	token := signIn(t, "plated", "plated-first-password")

	// /me is reachable before the forced change, which is what makes this
	// assertable without changing the password first.
	me := expectAs(t, token).GET("/me").Expect().
		Status(http.StatusOK).JSON().Object()
	me.Value("plates").Array().NotEmpty()
	me.Value("barWeightLb").Number().Gt(0)
}

func TestAdminCreateRejectsADuplicateUsername(t *testing.T) {
	createAccount(t, "twice", "twice-first-password")

	e := expect(t)
	e.POST("/admin/users").
		WithJSON(map[string]any{"username": "twice", "password": "another-password"}).
		Expect().Status(http.StatusConflict).
		JSON().Object().HasValue("code", "username_taken")

	// Usernames are compared case-insensitively at login, so they have to
	// collide case-insensitively here too — otherwise "Twice" registers and
	// then cannot be told apart from "twice" at the door.
	e.POST("/admin/users").
		WithJSON(map[string]any{"username": "TWICE", "password": "another-password"}).
		Expect().Status(http.StatusConflict).
		JSON().Object().HasValue("code", "username_taken")

	// The owner's own username is no different.
	e.POST("/admin/users").
		WithJSON(map[string]any{"username": primaryUsername, "password": "another-password"}).
		Expect().Status(http.StatusConflict).
		JSON().Object().HasValue("code", "username_taken")
}

// The same rules registration applies, from the same validators — an account
// this accepted but the sign-in form could not match would be a dead account.
func TestAdminCreateValidatesCredentials(t *testing.T) {
	e := expect(t)

	tests := map[string]map[string]any{
		"username too short":    {"username": "ab", "password": "long-enough-pw"},
		"username illegal char": {"username": "with space", "password": "long-enough-pw"},
		"password too short":    {"username": "shortpw", "password": "seven77"},
		"missing username":      {"password": "long-enough-pw"},
		"missing password":      {"username": "nopassword"},
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			e.POST("/admin/users").WithJSON(body).
				Expect().Status(http.StatusBadRequest).
				JSON().Object().HasValue("code", "bad_request")
		})
	}

	// Over the 64-character display-name cap.
	e.POST("/admin/users").
		WithJSON(map[string]any{
			"username":    "longname",
			"password":    "long-enough-pw",
			"displayName": string(make([]byte, 0, 70)) + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}).
		Expect().Status(http.StatusBadRequest)
}

// ---- the forced password change ----

// An account created by the admin is holding a password its owner did not
// choose and somebody else still knows. Until it replaces that password it may
// read /me and write /me/password, and nothing else.
func TestCreatedAccountIsGatedUntilItChangesItsPassword(t *testing.T) {
	createAccount(t, "gated", "gated-first-password")
	token := signIn(t, "gated", "gated-first-password")
	e := expectAs(t, token)

	// Reachable: this is how the client finds out it is gated at all.
	e.GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("mustChangePassword", true)

	// Signing in must not clear the flag. login() re-hashes opportunistically
	// on a successful verify, and if that used the same query as a real change
	// the account would escape the gate by USING the one-time password rather
	// than replacing it.
	e.GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("mustChangePassword", true)

	// Refused, across a read, a write, and the export — the export mounts
	// outside the authenticated group and so inherits nothing, which is exactly
	// the way a gate gets forgotten.
	for _, blocked := range []*httpexpect.Request{
		e.GET("/sessions"),
		e.GET("/programs"),
		e.GET("/racked"),
		e.GET("/me/export"),
		e.PATCH("/me").WithJSON(map[string]any{"displayName": "Sneaky"}),
	} {
		blocked.Expect().
			Status(http.StatusForbidden).
			JSON().Object().HasValue("code", "password_change_required")
	}

	// The way out.
	e.PUT("/me/password").
		WithJSON(map[string]any{
			"currentPassword": "gated-first-password",
			"newPassword":     "gated-chosen-password",
		}).
		Expect().Status(http.StatusNoContent)

	// The same session, now unblocked: changing the password revokes every
	// OTHER login but keeps this one, so the lifter is not bounced to the
	// sign-in form the moment they comply.
	e.GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("mustChangePassword", false)
	e.GET("/sessions").Expect().Status(http.StatusOK)
	e.GET("/me/export").Expect().Status(http.StatusOK)

	// And it is the new password that works from here.
	expectAnon(t).POST("/auth/login").
		WithJSON(map[string]any{"username": "gated", "password": "gated-first-password"}).
		Expect().Status(http.StatusUnauthorized)
	signIn(t, "gated", "gated-chosen-password")
}

// A gated account can still sign out. Nobody should be stuck in an app they
// cannot use and cannot leave, and /auth/logout is outside the gate for that
// reason alone.
func TestGatedAccountCanStillSignOut(t *testing.T) {
	createAccount(t, "leaver", "leaver-first-password")
	token := signIn(t, "leaver", "leaver-first-password")

	expectAs(t, token).POST("/auth/logout").Expect().Status(http.StatusNoContent)
	// The session is revoked server-side, not merely forgotten by the client.
	expectAs(t, token).GET("/me").Expect().Status(http.StatusUnauthorized)
}

// The owner is unaffected: they chose their own password at registration, so
// no gate applies and the app works as it always did.
func TestTheOwnerIsNotGated(t *testing.T) {
	e := expect(t)
	e.GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("mustChangePassword", false)
	e.GET("/sessions").Expect().Status(http.StatusOK)
}
