package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Account management for the install's owner.
//
// Self-registration is first-user-only and closes permanently (see register),
// which left no way to add a second account at all — the only route in was an
// INSERT by hand. These two endpoints are that route, and they are the only
// place in the app where one account acts on another, which is why they sit
// behind requireAdmin on their own subtree rather than among the per-user
// handlers.

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

// listUsers is the roster: every account on this install, oldest first.
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.q.ListUsers(r.Context())
	if err != nil {
		internalError(w)
		return
	}

	// Never nil. An install always has at least the owner reading this, so an
	// empty array is unreachable in practice — but a nil slice serializes as
	// null, and a client that has to handle both null and [] for "no rows" is
	// being asked to care about a distinction the API does not mean.
	users := make([]adminUserDTO, 0, len(rows))
	for _, row := range rows {
		users = append(users, adminUserDTO{
			ID:                 row.ID,
			Username:           row.Username,
			DisplayName:        row.DisplayName,
			AvatarColor:        row.AvatarColor,
			IsAdmin:            row.IsAdmin,
			MustChangePassword: row.MustChangePassword,
			CreatedAt:          timestamptzToString(row.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, users)
}

// createUser adds an ordinary account.
//
// Two things are not negotiable by the caller and so are not in the request
// body at all. The account is never an admin: users_single_admin_idx permits
// exactly one, the owner already holds it, and accepting a field whose only
// valid value is false invites a 409 for asking a reasonable-looking question.
// And it always starts with a forced password change, because its first
// password was typed by somebody who will never use the account — see
// 0021_must_change_password.
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	// The same rules registration applies, from the same functions: an account
	// made here signs in through the same form as the one made there, and a
	// username this accepted but login could not match would be a dead account.
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

	user, err := qtx.CreateUser(ctx, store.CreateUserParams{
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: hash,
		IsAdmin:      false,
		// See the doc comment: the password in this request was chosen by the
		// admin, not by the lifter who will use it.
		MustChangePassword: true,
	})
	if err != nil {
		// The username index, almost certainly — it is case-insensitive, so
		// "Ada" collides with "ada" and the admin needs to be told that rather
		// than shown a 500. No pre-check to race: the constraint is the arbiter
		// and losing to it is a 409 like any other.
		if isUniqueViolation(err) {
			conflict(w, "username_taken", "that username is already taken")
			return
		}
		internalError(w)
		return
	}

	// Same transaction as the account, exactly as register does it. This is
	// what lets an empty inventory mean "owns no plates" rather than "never
	// configured" (see 0013_gym_setup): an account either has a rack or does
	// not exist. Skipping it here would give admin-created lifters a subtly
	// different app from the owner's — bar-only plate maths, silently.
	if err := qtx.SeedDefaultPlates(ctx, user.ID); err != nil {
		internalError(w)
		return
	}

	// Deliberately no AdoptOrphanSessions. Unowned sessions predate accounts
	// existing at all, and register hands them to the account that claims the
	// install; there is nothing left for a later account to adopt, and doing it
	// again would move the owner's history onto somebody else.

	// Tell everybody already here. In the same transaction as the account, so
	// an account that exists has been announced and one that was rolled back
	// was never mentioned.
	//
	// This is the only place in the API a 'joined' notification is raised.
	// register does NOT do it, and that is not an omission: registration is open
	// only while the install has no accounts, so the lifter it creates is the
	// first one and there is nobody to tell. Calling it there would be a query
	// that can only ever select zero rows.
	if err := qtx.CreateJoinNotifications(ctx, user.ID); err != nil {
		internalError(w)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusCreated, adminUserDTO{
		ID:                 user.ID,
		Username:           user.Username,
		DisplayName:        user.DisplayName,
		AvatarColor:        user.AvatarColor,
		IsAdmin:            user.IsAdmin,
		MustChangePassword: user.MustChangePassword,
		CreatedAt:          timestamptzToString(user.CreatedAt),
	})
}
