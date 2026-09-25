package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Houses: a named group of lifters, and the sigil its members wear.
//
// # WHY NOTHING HERE FILTERS A READ
//
// lifters.go explains at length that this install has no per-visibility filter
// and that its absence is a decision: admission to the install is the consent.
// A House is exactly the feature that would have reversed that premise —
// members see each other, strangers do not — and it deliberately does not.
// Which House a lifter is in is as public as the name it is drawn beside, and
// not one query outside this file gained a house_id predicate.
//
// So a House groups; it does not gate. The shared achievements a House is for
// are the ones /achievements already serves to everybody: the House page
// composes that answer with its member list rather than asking a new question.
//
// # WHERE THE WRITES LIVE, AND WHY NOT UNDER /lifters
//
// /lifters is read-only by construction and may never write — see the note at
// the top of lifters.go, which says that a social feature needing a write
// belongs on the resource being written, where the caller is the authenticated
// user and the id in the path is not a person. That is why joining is
// POST /houses/{id}/requests and leaving is DELETE /me/house, and why neither
// takes a lifter id at all.
//
// # OWNERSHIP, AND THE ONE STATE THE SCHEMA CANNOT HOLD
//
// The founder owns the House. Ownership is a flag on the membership row rather
// than a column on houses, so an owner who is not a member cannot be
// represented and an account being deleted takes its ownership with it.
//
// What that cannot express is who owns a House whose owner deleted their
// account: the cascade removes the row and no transfer runs. Rather than a
// repair pass looking for that state, it is resolved by reading —
// GetHouseOwner orders by is_owner first and falls through to the
// longest-standing member, which is the lifter a transfer would have chosen
// anyway. Every owner-only check in this file goes through GetHouseOwner, so
// there is one rule and one place it is applied.
//
// # THE EMPTY HOUSE
//
// GetHouseOwner returns NO ROW for a House with no members, and that is an
// ordinary state rather than an impossible one: a House outlives its last
// member. It keeps its name, its sigil, its founding date and its description,
// all of which belonged to more lifters than the one who happened to leave
// last, and both name and sigil are indexed install-wide so deleting it is the
// only way anybody could reuse them.
//
// An empty House is therefore standing and unowned. The only way back in is
// claimEmptyHouse: the first lifter to ask joins it outright, as owner, because
// the ordinary path files a request for a human to approve and there is no
// human. Owner-only endpoints 404 in the meantime, which is honest — there is
// nobody they could be addressed to.

// The three notification kinds this feature raises.
//
// Written as constants because CreateHouseNotification takes the kind as a
// parameter — one statement serves all three, since they differ only in who is
// told. Every other fan-out in this repo writes its kind as a SQL literal
// instead, which is the right call when a query serves exactly one.
const (
	notificationKindHouseRequest  = "house-request"
	notificationKindHouseApproved = "house-approved"
	notificationKindHouseDeclined = "house-declined"
)

// The outcomes a join request can be closed with. Only 'superseded' is written
// anywhere but here — see SupersedeOtherOpenRequests, which sets it as a literal
// because it is the one outcome no lifter chooses.
const (
	houseRequestApproved  = "approved"
	houseRequestDeclined  = "declined"
	houseRequestWithdrawn = "withdrawn"
)

const (
	houseNameMaxRunes   = 60
	houseTaglineMax     = 80
	houseDescriptionMax = 2000
)

// houseSigilPattern is the same shape houses_sigil_ck enforces in 0034.
//
// Two copies of one rule, and the duplication is deliberate: the CHECK is what
// makes the rule true of the data whatever writes it, and this is what makes a
// bad sigil a 400 naming the field rather than a 500 from a constraint breach.
var houseSigilPattern = regexp.MustCompile(`^[A-Za-z0-9]{2,5}$`)

// houseIcons is the closed set the HouseIcon enum in openapi.yaml declares.
//
// The empty string is a member: a House founded without an icon has one, and
// the UI falls back to drawing the sigil. Kept in step with the spec by hand,
// which is the same arrangement ReactionEmoji already has — the alternative is
// a free string that renders as nothing when it is misspelled.
var houseIcons = map[string]struct{}{
	"":         {},
	"dumbbell": {},
	"flame":    {},
	"anvil":    {},
	"hammer":   {},
	"mountain": {},
	"shield":   {},
	"swords":   {},
	"skull":    {},
	"zap":      {},
	"star":     {},
	"gem":      {},
	"landmark": {},
	"castle":   {},
	"trophy":   {},
	"target":   {},
	"anchor":   {},
}

// ---- reading ----

// listHouses is the site-wide read every client keeps in order to draw a sigil
// beside any name it renders.
//
// Two queries and no join, because the client wants them apart: the houses draw
// the list screen and the hover cards, and the memberships become a map keyed by
// lifter id. Folding the second into the first as an array per House would mean
// every client walking every member array on every load to rebuild that map.
func (s *Server) listHouses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := s.q.ListHouses(ctx)
	if err != nil {
		internalError(w)
		return
	}
	memberships, err := s.q.ListHouseMemberships(ctx)
	if err != nil {
		internalError(w)
		return
	}

	items := make([]houseDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, houseDTO{
			ID:          row.ID,
			Name:        row.Name,
			Sigil:       row.Sigil,
			Tagline:     row.Tagline,
			Icon:        row.Icon,
			IconColor:   row.IconColor,
			CreatedAt:   timestamptzToString(row.CreatedAt),
			MemberCount: row.MemberCount,
		})
	}

	mships := make([]houseMembershipDTO, 0, len(memberships))
	for _, m := range memberships {
		mships = append(mships, houseMembershipDTO{
			UserID:  m.UserID,
			HouseID: m.HouseID,
			IsOwner: m.IsOwner,
		})
	}

	writeJSON(w, http.StatusOK, houseListDTO{Items: items, Memberships: mships})
}

// getHouse is one House in full, for its own page.
func (s *Server) getHouse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller := userFrom(ctx).ID

	house, ok := s.houseFromPath(w, r)
	if !ok {
		return
	}

	detail, ok := s.houseDetail(ctx, w, house, caller)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// ---- writing ----

type createHouseRequestBody struct {
	Name        string `json:"name"`
	Sigil       string `json:"sigil"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	IconColor   string `json:"iconColor"`
}

// createHouse founds one, with the caller as its first member and its owner.
func (s *Server) createHouse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller := userFrom(ctx).ID

	var body createHouseRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	body.Name = strings.TrimSpace(body.Name)
	body.Sigil = strings.TrimSpace(body.Sigil)
	body.Tagline = strings.TrimSpace(body.Tagline)
	if msg, ok := validateHouseFields(
		body.Name, body.Sigil, body.Tagline, body.Description, body.Icon, body.IconColor,
	); !ok {
		badRequest(w, msg)
		return
	}

	// Checked before the insert rather than left to the membership table's
	// primary key, because the collision would otherwise surface as a failure to
	// add the founder to a House that had already been created — and rolling that
	// back is correct but tells the lifter nothing.
	if _, err := s.q.GetHouseMembership(ctx, caller); err == nil {
		conflict(w, "already_in_house", "leave your current House before founding another")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
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

	house, err := qtx.CreateHouse(ctx, store.CreateHouseParams{
		Name:        body.Name,
		Sigil:       body.Sigil,
		Tagline:     body.Tagline,
		Description: body.Description,
		Icon:        body.Icon,
		IconColor:   body.IconColor,
	})
	if err != nil {
		// Both indexes are case-insensitive, so "Iron" collides with "iron" and
		// the founder needs to be told which field rather than shown a 500. No
		// pre-check to race: the constraint is the arbiter.
		if isUniqueViolation(err) {
			conflict(w, "house_exists", "a House already has that name or sigil")
			return
		}
		internalError(w)
		return
	}

	added, err := qtx.AddHouseMember(ctx, store.AddHouseMemberParams{
		UserID:  caller,
		HouseID: house.ID,
		IsOwner: true,
	})
	if err != nil {
		internalError(w)
		return
	}
	if added == 0 {
		// Lost the race with another request that joined this lifter to a House
		// between the check above and here. The House just created goes back with
		// the rollback, which is why both statements are in one transaction.
		conflict(w, "already_in_house", "leave your current House before founding another")
		return
	}

	// Founding answers the caller's own outstanding requests, for the reason
	// approving one does: they now have a House, so nobody else can let them in,
	// and leaving those pending would put an unanswerable row in another owner's
	// queue. exceptID 0 excludes nothing — there is no request being approved
	// here, so every one of theirs is superseded.
	if _, err := qtx.SupersedeOtherOpenRequests(ctx, store.SupersedeOtherOpenRequestsParams{
		UserID:   caller,
		ExceptID: 0,
	}); err != nil {
		internalError(w)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	detail, ok := s.houseDetail(ctx, w, house, caller)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

type updateHouseRequestBody struct {
	Name        *string `json:"name"`
	Sigil       *string `json:"sigil"`
	Tagline     *string `json:"tagline"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	IconColor   *string `json:"iconColor"`
}

// updateHouse rewrites the identity half of a House. Owner only.
//
// Pointers, so an omitted field is left alone and an empty string is a change —
// which is how a tagline is cleared. A plain string struct could not tell the
// two apart, and "PATCH with one field" would silently blank the other five.
func (s *Server) updateHouse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller := userFrom(ctx).ID

	house, ok := s.houseFromPath(w, r)
	if !ok {
		return
	}
	if !s.requireHouseOwner(ctx, w, house.ID, caller) {
		return
	}

	var body updateHouseRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	next := store.UpdateHouseParams{
		ID:          house.ID,
		Name:        house.Name,
		Sigil:       house.Sigil,
		Tagline:     house.Tagline,
		Description: house.Description,
		Icon:        house.Icon,
		IconColor:   house.IconColor,
	}
	if body.Name != nil {
		next.Name = strings.TrimSpace(*body.Name)
	}
	if body.Sigil != nil {
		next.Sigil = strings.TrimSpace(*body.Sigil)
	}
	if body.Tagline != nil {
		next.Tagline = strings.TrimSpace(*body.Tagline)
	}
	if body.Description != nil {
		next.Description = *body.Description
	}
	if body.Icon != nil {
		next.Icon = *body.Icon
	}
	if body.IconColor != nil {
		next.IconColor = *body.IconColor
	}

	if msg, ok := validateHouseFields(
		next.Name, next.Sigil, next.Tagline, next.Description, next.Icon, next.IconColor,
	); !ok {
		badRequest(w, msg)
		return
	}

	updated, err := s.q.UpdateHouse(ctx, next)
	if err != nil {
		if isUniqueViolation(err) {
			conflict(w, "house_exists", "a House already has that name or sigil")
			return
		}
		internalError(w)
		return
	}

	detail, ok := s.houseDetail(ctx, w, updated, caller)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// requestToJoinHouse files the caller's ask. The owner is notified.
func (s *Server) requestToJoinHouse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// The whole caller rather than just their id, because the response carries a
	// Lifter and the spec requires its name fields. They are already on the
	// context, so filling them costs nothing — and a request echoed back with an
	// empty username would be a shape no other endpoint sends.
	me := userFrom(ctx)
	caller := me.ID

	house, ok := s.houseFromPath(w, r)
	if !ok {
		return
	}

	if _, err := s.q.GetHouseMembership(ctx, caller); err == nil {
		conflict(w, "already_in_house", "leave your current House before asking to join another")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
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

	// The owner read decides which of this endpoint's two outcomes happens, so it
	// is taken INSIDE the transaction and under the House's row lock. Read
	// unlocked, two lifters asking the same empty House at once would both see no
	// owner and both claim it — see LockHouse for why AddHouseMember cannot catch
	// that pair.
	if _, err := qtx.LockHouse(ctx, house.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Deleted between houseFromPath and here. Nothing left to join.
			notFound(w, "House not found")
			return
		}
		internalError(w)
		return
	}

	// A House outlives its last member, so "no owner" is an ordinary state rather
	// than the unreachable one it used to be: the House is standing, nobody is in
	// it, and there is no one to work a queue. Filing a request against it would
	// put a row somewhere nobody will ever look, so the caller walks in instead.
	owner, err := qtx.GetHouseOwner(ctx, house.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		s.claimEmptyHouse(ctx, w, tx, qtx, house, caller)
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	req, err := qtx.CreateJoinRequest(ctx, store.CreateJoinRequestParams{
		HouseID: house.ID,
		UserID:  caller,
	})
	if err != nil {
		// house_join_requests_open_idx: one OPEN request per lifter per House. A
		// second ask while the first is pending is a 409; asking again after a
		// decline is an ordinary insert, because the index is partial.
		if isUniqueViolation(err) {
			conflict(w, "already_requested", "you have already asked to join this House")
			return
		}
		internalError(w)
		return
	}

	told, err := qtx.CreateHouseNotification(ctx, store.CreateHouseNotificationParams{
		UserID:  owner.UserID,
		ActorID: caller,
		Kind:    notificationKindHouseRequest,
		HouseID: house.ID,
	})
	if err != nil {
		internalError(w)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	// After the commit, never in a defer — see the note on liveEvents.publish.
	events := s.newLiveEvents()
	events.notify(told)
	events.publish()

	writeJSON(w, http.StatusCreated, houseJoinRequestDTO{
		ID:      req.ID,
		HouseID: req.HouseID,
		Lifter: lifterDTO{
			ID:          caller,
			Username:    me.Username,
			DisplayName: me.DisplayName,
			AvatarColor: me.AvatarColor,
		},
		RequestedAt: timestamptzToString(req.RequestedAt),
	})
}

// claimEmptyHouse walks a lifter into a House that has no members left, as its
// owner, and answers with the House rather than with a request.
//
// The asking endpoint's other path files a row and waits for a human. This one
// cannot: there is no human. An empty House would otherwise be a headstone —
// standing, unjoinable, and holding a name and a sigil nobody else can use,
// because both indexes are install-wide. So the first lifter to ask takes it.
//
// Answering 200-with-a-House rather than 201-with-a-request is what tells the
// client which of the two happened, without a second round trip to find out.
//
// Takes the caller's transaction rather than opening its own, because the read
// that chose this path — "no owner" — is only true for as long as that
// transaction holds the House's row lock. Beginning a second one here would put
// the decision and the write in different transactions, which is the race the
// lock exists to close.
func (s *Server) claimEmptyHouse(
	ctx context.Context,
	w http.ResponseWriter,
	tx pgx.Tx,
	qtx *store.Queries,
	house store.House,
	caller int32,
) {
	added, err := qtx.AddHouseMember(ctx, store.AddHouseMemberParams{
		UserID:  caller,
		HouseID: house.ID,
		IsOwner: true,
	})
	if err != nil {
		internalError(w)
		return
	}
	if added == 0 {
		// Lost the race with a request that joined this lifter to a House between
		// the membership check above and here — the same 409 founding a House
		// answers, and for the same reason.
		conflict(w, "already_in_house", "leave your current House before asking to join another")
		return
	}

	// Joining answers the caller's own outstanding requests elsewhere, exactly as
	// founding does: they have a House now, so the queues they are sitting in
	// cannot be worked. ExceptID 0 excludes nothing — no request was approved
	// here, because none was ever filed.
	if _, err := qtx.SupersedeOtherOpenRequests(ctx, store.SupersedeOtherOpenRequestsParams{
		UserID:   caller,
		ExceptID: 0,
	}); err != nil {
		internalError(w)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	// Nobody is notified. The notification on the other path addresses the owner,
	// and here the caller IS the owner — telling them what they just did is noise.

	detail, ok := s.houseDetail(ctx, w, house, caller)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// withdrawHouseRequest closes the caller's own pending request.
//
// No notification. The owner was told it arrived, and being told it went away
// again is not news — it is the absence of a row in a list they may never have
// opened.
func (s *Server) withdrawHouseRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller := userFrom(ctx).ID

	house, ok := s.houseFromPath(w, r)
	if !ok {
		return
	}
	req, ok := s.pendingRequestFromPath(w, r, house.ID)
	if !ok {
		return
	}

	// Somebody else's request is a 404 rather than a 403: a lifter has no
	// business learning that a request they did not make exists.
	if req.UserID != caller {
		notFound(w, "request not found")
		return
	}

	decided, err := s.q.DecideJoinRequest(ctx, store.DecideJoinRequestParams{
		ID:        req.ID,
		Outcome:   houseRequestWithdrawn,
		DecidedBy: &caller,
	})
	if err != nil {
		internalError(w)
		return
	}
	if decided == 0 {
		notFound(w, "request not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// approveHouseRequest lets a lifter in. Owner only.
func (s *Server) approveHouseRequest(w http.ResponseWriter, r *http.Request) {
	s.decideHouseRequest(w, r, houseRequestApproved)
}

// declineHouseRequest turns one down. Owner only.
//
// A declined request is closed, not a bar: the index that stops a second ask is
// partial on the pending ones, so the lifter may try again later.
func (s *Server) declineHouseRequest(w http.ResponseWriter, r *http.Request) {
	s.decideHouseRequest(w, r, houseRequestDeclined)
}

// decideHouseRequest is both owner decisions, which differ only in whether a
// membership is written and which sentence the requester is sent.
func (s *Server) decideHouseRequest(w http.ResponseWriter, r *http.Request, outcome string) {
	ctx := r.Context()
	caller := userFrom(ctx).ID

	house, ok := s.houseFromPath(w, r)
	if !ok {
		return
	}
	if !s.requireHouseOwner(ctx, w, house.ID, caller) {
		return
	}
	req, ok := s.pendingRequestFromPath(w, r, house.ID)
	if !ok {
		return
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	// Closed first, and the query's own `decided_at IS NULL` guard is what makes
	// a double tap idempotent: the second call updates nothing, reports zero, and
	// leaves the first decision standing rather than overwriting an approval with
	// a decline because two taps raced.
	decided, err := qtx.DecideJoinRequest(ctx, store.DecideJoinRequestParams{
		ID:        req.ID,
		Outcome:   outcome,
		DecidedBy: &caller,
	})
	if err != nil {
		internalError(w)
		return
	}
	if decided == 0 {
		conflict(w, "already_decided", "that request has already been decided")
		return
	}

	kind := notificationKindHouseDeclined
	if outcome == houseRequestApproved {
		kind = notificationKindHouseApproved

		added, err := qtx.AddHouseMember(ctx, store.AddHouseMemberParams{
			UserID:  req.UserID,
			HouseID: house.ID,
			IsOwner: false,
		})
		if err != nil {
			internalError(w)
			return
		}
		if added == 0 {
			// They joined somewhere else while this sat in the queue. Failing is
			// the kind answer: AddHouseMember declines to relocate them on
			// purpose, because quietly moving a lifter out of the House they
			// chose would be worse than telling their would-be owner no.
			conflict(w, "already_in_house", "that lifter has since joined another House")
			return
		}

		// Their other asks are now unanswerable, so they are closed here rather
		// than left to fail one by one in other owners' queues.
		if _, err := qtx.SupersedeOtherOpenRequests(ctx, store.SupersedeOtherOpenRequestsParams{
			UserID:   req.UserID,
			ExceptID: req.ID,
		}); err != nil {
			internalError(w)
			return
		}
	}

	told, err := qtx.CreateHouseNotification(ctx, store.CreateHouseNotificationParams{
		UserID:  req.UserID,
		ActorID: caller,
		Kind:    kind,
		HouseID: house.ID,
	})
	if err != nil {
		internalError(w)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	events := s.newLiveEvents()
	events.notify(told)
	events.publish()

	w.WriteHeader(http.StatusNoContent)
}

// leaveHouse removes the caller from the House they are in.
//
// Two things can follow and neither is the caller's choice. If they owned it and
// others remain, ownership passes to the longest-standing member — a House is
// not left ownerless because one lifter walked. If they were the last member the
// House STANDS, empty and unowned, keeping its name, sigil, founding date and
// description; claimEmptyHouse is the way back into one.
//
// Its outstanding requests are superseded here rather than cascading away with a
// deleted House. Nobody is left to answer them, and leaving them open would lock
// their own askers out of claiming it — see SupersedeHouseOpenRequests.
func (s *Server) leaveHouse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller := userFrom(ctx).ID

	membership, err := s.q.GetHouseMembership(ctx, caller)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "you are not in a House")
		return
	}
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

	if _, err := qtx.RemoveHouseMember(ctx, caller); err != nil {
		internalError(w)
		return
	}

	// A House is NOT deleted when its last member goes. It stands empty, keeping
	// its name, its sigil, its founding date and whatever its members wrote about
	// it, and the first lifter to ask walks in as its owner — see claimEmptyHouse.
	// Deleting it would throw all of that away on the say-so of one lifter who was
	// merely the last to leave.
	//
	// So the only thing to settle here is the owner, and only while somebody is
	// left to hold it. Asked rather than assumed from membership.IsOwner, so that
	// a House which was ALREADY ownerless — its owner deleted their account, and
	// the cascade took the membership row before any transfer could run — gets a
	// marked owner the first time anybody leaves it. GetHouseOwner returns the
	// fallback with IsOwner false, which is exactly the signal to promote.
	owner, err := qtx.GetHouseOwner(ctx, membership.HouseID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// That was the last member. Nobody to promote — but every request still
		// open against this House has just become unanswerable, and those rows no
		// longer cascade away with a deleted House. See the query's note: left
		// open, they would lock their own askers out of claiming it.
		if _, err := qtx.SupersedeHouseOpenRequests(ctx, membership.HouseID); err != nil {
			internalError(w)
			return
		}
	case err != nil:
		internalError(w)
		return
	case !owner.IsOwner:
		if _, err := qtx.PromoteLongestStandingMember(ctx, membership.HouseID); err != nil {
			internalError(w)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---- helpers ----

// houseFromPath reads the {houseId} parameter and loads the House.
func (s *Server) houseFromPath(w http.ResponseWriter, r *http.Request) (store.House, bool) {
	id, ok := idParam(r, "houseId")
	if !ok {
		notFound(w, "House not found")
		return store.House{}, false
	}
	house, err := s.q.GetHouse(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "House not found")
		return store.House{}, false
	}
	if err != nil {
		internalError(w)
		return store.House{}, false
	}
	return house, true
}

// pendingRequestFromPath reads {requestId} and returns it only if it is still
// open AND belongs to the House in the path.
//
// The House check is what stops an id from another House being decided by this
// House's owner. A mismatch is a 404 rather than a 403 for the reason the
// withdraw path gives: the caller is not entitled to know the row exists.
func (s *Server) pendingRequestFromPath(
	w http.ResponseWriter, r *http.Request, houseID int32,
) (store.GetJoinRequestRow, bool) {
	id, ok := idParam(r, "requestId")
	if !ok {
		notFound(w, "request not found")
		return store.GetJoinRequestRow{}, false
	}
	req, err := s.q.GetJoinRequest(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "request not found")
		return store.GetJoinRequestRow{}, false
	}
	if err != nil {
		internalError(w)
		return store.GetJoinRequestRow{}, false
	}
	if req.HouseID != houseID || req.DecidedAt.Valid {
		notFound(w, "request not found")
		return store.GetJoinRequestRow{}, false
	}
	return req, true
}

// requireHouseOwner answers the owner-only endpoints' question.
//
// A member who is not the owner gets 403; a lifter who is not in this House at
// all gets 404, because the membership is not theirs to know about.
func (s *Server) requireHouseOwner(
	ctx context.Context, w http.ResponseWriter, houseID, caller int32,
) bool {
	owner, err := s.q.GetHouseOwner(ctx, houseID)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "House not found")
		return false
	}
	if err != nil {
		internalError(w)
		return false
	}
	if owner.UserID == caller {
		return true
	}

	membership, err := s.q.GetHouseMembership(ctx, caller)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		internalError(w)
		return false
	}
	if err == nil && membership.HouseID == houseID {
		forbidden(w, "house_owner_required", "only the House's owner can do that")
		return false
	}
	notFound(w, "House not found")
	return false
}

// houseDetail builds the detail response, including the caller's own standing.
func (s *Server) houseDetail(
	ctx context.Context, w http.ResponseWriter, house store.House, caller int32,
) (houseDetailDTO, bool) {
	memberRows, err := s.q.ListHouseMembers(ctx, house.ID)
	if err != nil {
		internalError(w)
		return houseDetailDTO{}, false
	}

	members := make([]houseMemberDTO, 0, len(memberRows))
	isMember := false
	for _, row := range memberRows {
		if row.ID == caller {
			isMember = true
		}
		members = append(members, houseMemberDTO{
			Lifter: lifterDTO{
				ID:            row.ID,
				Username:      row.Username,
				DisplayName:   row.DisplayName,
				AvatarColor:   row.AvatarColor,
				HasAvatar:     row.AvatarEtag != "",
				AvatarEtag:    row.AvatarEtag,
				LastTrainedOn: dateToString(row.LastTrainedOn),
			},
			IsOwner:  row.IsOwner,
			JoinedAt: timestamptzToString(row.JoinedAt),
		})
	}

	viewer := houseViewerDTO{IsMember: isMember}

	if isMember {
		owner, err := s.q.GetHouseOwner(ctx, house.ID)
		if err != nil {
			internalError(w)
			return houseDetailDTO{}, false
		}
		viewer.IsOwner = owner.UserID == caller
	} else {
		// Only asked for a non-member, because a member is by definition not in
		// another House — one House at a time is the membership table's primary
		// key, not a rule this has to re-check.
		membership, err := s.q.GetHouseMembership(ctx, caller)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			internalError(w)
			return houseDetailDTO{}, false
		}
		viewer.InAnotherHouse = err == nil && membership.HouseID != house.ID

		open, err := s.q.GetOpenRequest(ctx, store.GetOpenRequestParams{
			HouseID: house.ID,
			UserID:  caller,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			internalError(w)
			return houseDetailDTO{}, false
		}
		if err == nil {
			id := open.ID
			viewer.OpenRequestID = &id
		}
	}

	detail := houseDetailDTO{
		ID:          house.ID,
		Name:        house.Name,
		Sigil:       house.Sigil,
		Tagline:     house.Tagline,
		Description: house.Description,
		Icon:        house.Icon,
		IconColor:   house.IconColor,
		CreatedAt:   timestamptzToString(house.CreatedAt),
		MemberCount: len(members),
		Members:     members,
		Viewer:      viewer,
	}

	// Only the owner is told who is waiting, and the pointer is what keeps
	// "nobody has asked" distinguishable from "you may not know" — see the field's
	// note on houseDetailDTO.
	if viewer.IsOwner {
		reqRows, err := s.q.ListPendingHouseRequests(ctx, house.ID)
		if err != nil {
			internalError(w)
			return houseDetailDTO{}, false
		}
		pending := make([]houseJoinRequestDTO, 0, len(reqRows))
		for _, row := range reqRows {
			pending = append(pending, houseJoinRequestDTO{
				ID:      row.ID,
				HouseID: house.ID,
				Lifter: lifterDTO{
					ID:          row.UserID,
					Username:    row.Username,
					DisplayName: row.DisplayName,
					AvatarColor: row.AvatarColor,
					HasAvatar:   row.AvatarEtag != "",
					AvatarEtag:  row.AvatarEtag,
				},
				RequestedAt: timestamptzToString(row.RequestedAt),
			})
		}
		detail.PendingRequests = &pending
	}

	return detail, true
}

// validateHouseFields checks everything a House's identity has to satisfy,
// returning the message for a 400 when it does not.
//
// One function for both the create and the update path, so a rule cannot be
// enforced on the way in and forgotten on the way through. Runes rather than
// bytes for the name, matching the CHECK constraints, which count characters.
func validateHouseFields(name, sigil, tagline, description, icon, iconColor string) (string, bool) {
	switch {
	case name == "":
		return "name is required", false
	case utf8.RuneCountInString(name) > houseNameMaxRunes:
		return "name is too long", false
	case !houseSigilPattern.MatchString(sigil):
		return "sigil must be 2 to 5 letters or digits", false
	case utf8.RuneCountInString(tagline) > houseTaglineMax:
		return "tagline is too long", false
	case utf8.RuneCountInString(description) > houseDescriptionMax:
		return "description is too long", false
	}
	if _, ok := houseIcons[icon]; !ok {
		return "unknown icon", false
	}
	// Reuses the profile's colour rule rather than restating it: the icon chip and
	// the initials chip are drawn by the same kind of code and a colour that is
	// valid for one has to be valid for the other.
	if iconColor != "" && !avatarColorPattern.MatchString(iconColor) {
		return "iconColor must be a #rrggbb hex colour", false
	}
	return "", true
}
