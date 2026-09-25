package api_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
)

// Houses: founding one, getting into one, and what happens when somebody leaves.
//
// WHY EVERY LIFTER HERE IS A secondLifter
//
// The primary account owns the install and is shared by the whole suite. Putting
// it in a House would give it a sigil for every test that runs after this file,
// and — worse — addressing a join request to it would change the unread counts
// the notification suite asserts on exactly. So nothing here touches it, and the
// Houses are torn down by id afterwards.
//
// The teardown is one DELETE per House and it is load-bearing rather than tidy:
// house_members, house_join_requests and notifications.house_id all cascade from
// houses, so removing the House removes every trace of the test including the
// notifications it raised. Deleting the accounts alone would not — a House
// founded by a deleted lifter survives on purpose.
//
// WHAT IS WORTH MOST HERE
//
// The rules the schema cannot state. One House at a time is a primary key and
// needs almost no testing; who may approve, what a first approval does to the
// requester's other asks, and who owns a House after the owner walks are all
// decisions in houses.go, and each of them is a state that is easy to get wrong
// in a way nothing else would notice.

// foundHouse creates a House as the given lifter and removes it afterwards.
func foundHouse(t *testing.T, token, name, sigil string) int {
	t.Helper()
	obj := expectAs(t, token).POST("/houses").
		WithJSON(map[string]any{"name": name, "sigil": sigil}).
		Expect().Status(http.StatusCreated).
		JSON().Object()
	id := int(obj.Value("id").Number().Raw())
	t.Cleanup(func() {
		if _, err := testPool.Exec(
			context.Background(), `DELETE FROM houses WHERE id = $1`, id,
		); err != nil {
			t.Errorf("deleting house %d: %v", id, err)
		}
	})
	return id
}

// askToJoin files a request and returns its id.
func askToJoin(t *testing.T, token string, houseID int) int {
	t.Helper()
	return int(expectAs(t, token).POST("/houses/{id}/requests", houseID).
		Expect().Status(http.StatusCreated).
		JSON().Object().Value("id").Number().Raw())
}

// findHouseNotification returns the caller's folded row of the given kind.
//
// By kind alone, and returning ok rather than failing, which is where it differs
// from findNotification in the notifications suite: none of the house kinds
// carries a session to key on, and half of what is worth asserting here is that a
// row is ABSENT — that nobody was told about their own action.
func findHouseNotification(t *testing.T, token, kind string) (*httpexpect.Object, bool) {
	t.Helper()
	for _, item := range notificationsFor(expectAs(t, token), "limit", 100).Iter() {
		obj := item.Object()
		if obj.Value("kind").String().Raw() == kind {
			return obj, true
		}
	}
	return nil, false
}

// memberIDs reads a House's members in the order the API returns them.
func memberIDs(t *testing.T, token string, houseID int) []int {
	t.Helper()
	members := expectAs(t, token).GET("/houses/{id}", houseID).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("members").Array()
	out := make([]int, 0, len(members.Iter()))
	for _, m := range members.Iter() {
		out = append(out, int(m.Object().Value("lifter").Object().Value("id").Number().Raw()))
	}
	return out
}

func TestFoundingAHouseMakesYouItsOwner(t *testing.T) {
	id, token := secondLifter(t, "house-founder")
	houseID := foundHouse(t, token, "House Founder", "HFND")

	detail := expectAs(t, token).GET("/houses/{id}", houseID).
		Expect().Status(http.StatusOK).JSON().Object()
	detail.Value("memberCount").Number().IsEqual(1)
	detail.Value("viewer").Object().Value("isMember").Boolean().IsTrue()
	detail.Value("viewer").Object().Value("isOwner").Boolean().IsTrue()

	member := detail.Value("members").Array().Value(0).Object()
	member.Value("isOwner").Boolean().IsTrue()
	member.Value("lifter").Object().Value("id").Number().IsEqual(id)

	// An owner with nobody waiting gets an EMPTY list, not an absent one — the
	// two answers mean different things and houseDetailDTO keeps them apart.
	detail.Value("pendingRequests").Array().IsEmpty()
}

func TestTheSiteWideListCarriesEveryHouseAndMembership(t *testing.T) {
	id, token := secondLifter(t, "house-listed")
	houseID := foundHouse(t, token, "House Listed", "HLST")

	body := expectAs(t, token).GET("/houses").
		Expect().Status(http.StatusOK).JSON().Object()

	var found bool
	for _, item := range body.Value("items").Array().Iter() {
		obj := item.Object()
		if int(obj.Value("id").Number().Raw()) != houseID {
			continue
		}
		found = true
		obj.Value("sigil").String().IsEqual("HLST")
		obj.Value("memberCount").Number().IsEqual(1)
		// The long field is not in the payload every client loads.
		obj.NotContainsKey("description")
	}
	if !found {
		t.Fatalf("house %d missing from /houses", houseID)
	}

	var mapped bool
	for _, m := range body.Value("memberships").Array().Iter() {
		obj := m.Object()
		if int(obj.Value("userId").Number().Raw()) == int(id) {
			mapped = true
			obj.Value("houseId").Number().IsEqual(houseID)
			obj.Value("isOwner").Boolean().IsTrue()
		}
	}
	if !mapped {
		t.Fatalf("membership for lifter %d missing from /houses", id)
	}
}

func TestAHouseNameAndSigilAreUniqueCaseInsensitively(t *testing.T) {
	_, first := secondLifter(t, "house-unique-a")
	_, second := secondLifter(t, "house-unique-b")
	foundHouse(t, first, "House Unique", "HUNQ")

	// Same name in a different case.
	expectAs(t, second).POST("/houses").
		WithJSON(map[string]any{"name": "house unique", "sigil": "OTHR"}).
		Expect().Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("house_exists")

	// Same sigil in a different case.
	expectAs(t, second).POST("/houses").
		WithJSON(map[string]any{"name": "Something Else", "sigil": "hunq"}).
		Expect().Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("house_exists")
}

func TestASigilMustBeTwoToFiveAlphanumerics(t *testing.T) {
	_, token := secondLifter(t, "house-sigil-shape")

	for _, sigil := range []string{"", "X", "TOOLONG", "AB-C", "AB C", "ÅÄÖ"} {
		expectAs(t, token).POST("/houses").
			WithJSON(map[string]any{"name": "House " + sigil, "sigil": sigil}).
			Expect().Status(http.StatusBadRequest)
	}
}

func TestATaglineAndDescriptionHaveCeilings(t *testing.T) {
	_, token := secondLifter(t, "house-prose-limits")

	expectAs(t, token).POST("/houses").
		WithJSON(map[string]any{
			"name": "House Tagline", "sigil": "HTAG",
			"tagline": strings.Repeat("x", 81),
		}).
		Expect().Status(http.StatusBadRequest)

	expectAs(t, token).POST("/houses").
		WithJSON(map[string]any{
			"name": "House Description", "sigil": "HDSC",
			"description": strings.Repeat("x", 2001),
		}).
		Expect().Status(http.StatusBadRequest)

	// Both at their ceiling are fine, which is the half of a boundary that a
	// test asserting only the rejection would leave unproven.
	obj := expectAs(t, token).POST("/houses").
		WithJSON(map[string]any{
			"name": "House At The Limit", "sigil": "HLIM",
			"tagline":     strings.Repeat("x", 80),
			"description": strings.Repeat("y", 2000),
		}).
		Expect().Status(http.StatusCreated).JSON().Object()
	houseID := int(obj.Value("id").Number().Raw())
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM houses WHERE id = $1`, houseID)
	})
}

func TestAnUnknownIconIsRejected(t *testing.T) {
	_, token := secondLifter(t, "house-bad-icon")

	expectAs(t, token).POST("/houses").
		WithJSON(map[string]any{
			"name": "House Bad Icon", "sigil": "HBIC", "icon": "barbell-of-doom",
		}).
		Expect().Status(http.StatusBadRequest)

	expectAs(t, token).POST("/houses").
		WithJSON(map[string]any{
			"name": "House Bad Colour", "sigil": "HBCL", "iconColor": "purple",
		}).
		Expect().Status(http.StatusBadRequest)
}

func TestALifterMayOnlyBeInOneHouse(t *testing.T) {
	_, owner := secondLifter(t, "house-one-at-a-time")
	_, other := secondLifter(t, "house-one-other")
	first := foundHouse(t, owner, "House First", "HFST")
	foundHouse(t, other, "House Second", "HSND")

	// Founding a second.
	expectAs(t, owner).POST("/houses").
		WithJSON(map[string]any{"name": "House Third", "sigil": "HTRD"}).
		Expect().Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("already_in_house")

	// Asking to join one while already in another.
	expectAs(t, other).POST("/houses/{id}/requests", first).
		Expect().Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("already_in_house")

	// And the page tells them why before they try.
	expectAs(t, other).GET("/houses/{id}", first).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("viewer").Object().
		Value("inAnotherHouse").Boolean().IsTrue()
}

func TestRequestingAndApprovingAddsTheMember(t *testing.T) {
	_, owner := secondLifter(t, "house-approve-owner")
	joinerID, joiner := secondLifter(t, "house-approve-joiner")
	houseID := foundHouse(t, owner, "House Approve", "HAPR")

	requestID := askToJoin(t, joiner, houseID)

	// Asking twice while the first is pending.
	expectAs(t, joiner).POST("/houses/{id}/requests", houseID).
		Expect().Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("already_requested")

	// The requester's own page offers to withdraw rather than to ask again.
	expectAs(t, joiner).GET("/houses/{id}", houseID).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("viewer").Object().
		Value("openRequestId").Number().IsEqual(requestID)

	expectAs(t, owner).
		POST("/houses/{h}/requests/{r}/approve", houseID, requestID).
		Expect().Status(http.StatusNoContent)

	members := memberIDs(t, owner, houseID)
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %v", members)
	}
	var joined bool
	for _, id := range members {
		if id == int(joinerID) {
			joined = true
		}
	}
	if !joined {
		t.Fatalf("lifter %d not among members %v", joinerID, members)
	}

	// Deciding the same request again.
	expectAs(t, owner).
		POST("/houses/{h}/requests/{r}/approve", houseID, requestID).
		Expect().Status(http.StatusNotFound)
}

func TestARequestTellsTheOwnerAndNotTheRequester(t *testing.T) {
	_, owner := secondLifter(t, "house-notify-owner")
	_, joiner := secondLifter(t, "house-notify-joiner")
	houseID := foundHouse(t, owner, "House Notify", "HNTF")

	askToJoin(t, joiner, houseID)

	row, ok := findHouseNotification(t, owner, "house-request")
	if !ok {
		t.Fatal("the owner was not told about the request")
	}
	row.Value("houseId").Number().IsEqual(houseID)

	if _, ok := findHouseNotification(t, joiner, "house-request"); ok {
		t.Fatal("the requester was told about their own request")
	}
}

func TestADecisionTellsTheRequesterAndNotTheOwner(t *testing.T) {
	_, owner := secondLifter(t, "house-decide-owner")
	_, joiner := secondLifter(t, "house-decide-joiner")
	houseID := foundHouse(t, owner, "House Decide", "HDCD")

	requestID := askToJoin(t, joiner, houseID)
	expectAs(t, owner).
		POST("/houses/{h}/requests/{r}/approve", houseID, requestID).
		Expect().Status(http.StatusNoContent)

	row, ok := findHouseNotification(t, joiner, "house-approved")
	if !ok {
		t.Fatal("the requester was not told they were let in")
	}
	row.Value("houseId").Number().IsEqual(houseID)

	if _, ok := findHouseNotification(t, owner, "house-approved"); ok {
		t.Fatal("the owner was told about their own decision")
	}
}

func TestDecliningClosesTheRequestAndAllowsAnother(t *testing.T) {
	_, owner := secondLifter(t, "house-decline-owner")
	_, joiner := secondLifter(t, "house-decline-joiner")
	houseID := foundHouse(t, owner, "House Decline", "HDCL")

	requestID := askToJoin(t, joiner, houseID)
	expectAs(t, owner).
		POST("/houses/{h}/requests/{r}/decline", houseID, requestID).
		Expect().Status(http.StatusNoContent)

	if _, ok := findHouseNotification(t, joiner, "house-declined"); !ok {
		t.Fatal("the requester was not told they were turned down")
	}

	// Still not a member, and free to ask again — the index that stops a second
	// ask is partial on the pending ones.
	expectAs(t, joiner).GET("/houses/{id}", houseID).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("viewer").Object().
		Value("isMember").Boolean().IsFalse()

	askToJoin(t, joiner, houseID)
}

func TestWithdrawingYourOwnRequestClosesIt(t *testing.T) {
	_, owner := secondLifter(t, "house-withdraw-owner")
	_, joiner := secondLifter(t, "house-withdraw-joiner")
	_, stranger := secondLifter(t, "house-withdraw-stranger")
	houseID := foundHouse(t, owner, "House Withdraw", "HWDR")

	requestID := askToJoin(t, joiner, houseID)

	// Somebody else's request is a 404, not a 403: a lifter has no business
	// learning that a request they did not make exists.
	expectAs(t, stranger).
		DELETE("/houses/{h}/requests/{r}", houseID, requestID).
		Expect().Status(http.StatusNotFound)

	expectAs(t, joiner).
		DELETE("/houses/{h}/requests/{r}", houseID, requestID).
		Expect().Status(http.StatusNoContent)

	expectAs(t, joiner).GET("/houses/{id}", houseID).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("viewer").Object().
		NotContainsKey("openRequestId")
}

func TestOnlyTheOwnerDecidesAndEdits(t *testing.T) {
	_, owner := secondLifter(t, "house-authority-owner")
	_, member := secondLifter(t, "house-authority-member")
	_, asker := secondLifter(t, "house-authority-asker")
	_, stranger := secondLifter(t, "house-authority-stranger")
	houseID := foundHouse(t, owner, "House Authority", "HAUT")

	// Get an ordinary member in.
	memberRequest := askToJoin(t, member, houseID)
	expectAs(t, owner).
		POST("/houses/{h}/requests/{r}/approve", houseID, memberRequest).
		Expect().Status(http.StatusNoContent)

	pending := askToJoin(t, asker, houseID)

	// A member who is not the owner is told which rule stopped them.
	expectAs(t, member).
		POST("/houses/{h}/requests/{r}/approve", houseID, pending).
		Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("house_owner_required")

	expectAs(t, member).PATCH("/houses/{id}", houseID).
		WithJSON(map[string]any{"tagline": "not yours to write"}).
		Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("house_owner_required")

	// A lifter outside the House gets a 404 instead: the membership is not
	// theirs to know about.
	expectAs(t, stranger).PATCH("/houses/{id}", houseID).
		WithJSON(map[string]any{"tagline": "nor yours"}).
		Expect().Status(http.StatusNotFound)

	// And a member cannot see who is waiting.
	expectAs(t, member).GET("/houses/{id}", houseID).
		Expect().Status(http.StatusOK).
		JSON().Object().NotContainsKey("pendingRequests")
}

func TestTheOwnerCanEditTheHousesIdentity(t *testing.T) {
	_, owner := secondLifter(t, "house-edit-owner")
	houseID := foundHouse(t, owner, "House Edit", "HEDT")

	updated := expectAs(t, owner).PATCH("/houses/{id}", houseID).
		WithJSON(map[string]any{
			"tagline":     "We lift at dawn",
			"description": "A longer account of who we are.",
			"icon":        "anvil",
			"iconColor":   "#b026ff",
		}).
		Expect().Status(http.StatusOK).JSON().Object()
	updated.Value("tagline").String().IsEqual("We lift at dawn")
	updated.Value("description").String().IsEqual("A longer account of who we are.")
	updated.Value("icon").String().IsEqual("anvil")
	updated.Value("iconColor").String().IsEqual("#b026ff")
	// Untouched fields are left alone, which is what the pointers in
	// updateHouseRequestBody are for.
	updated.Value("name").String().IsEqual("House Edit")
	updated.Value("sigil").String().IsEqual("HEDT")

	// An empty string is a change, not an omission — this is how a tagline goes
	// away again.
	expectAs(t, owner).PATCH("/houses/{id}", houseID).
		WithJSON(map[string]any{"tagline": ""}).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("tagline").String().IsEqual("")
}

func TestApprovingSupersedesTheLiftersOtherRequests(t *testing.T) {
	_, firstOwner := secondLifter(t, "house-supersede-a")
	_, secondOwner := secondLifter(t, "house-supersede-b")
	_, joiner := secondLifter(t, "house-supersede-joiner")
	houseA := foundHouse(t, firstOwner, "House Supersede A", "HSPA")
	houseB := foundHouse(t, secondOwner, "House Supersede B", "HSPB")

	// Asking two Houses at once is deliberate behaviour, not a loophole.
	requestA := askToJoin(t, joiner, houseA)
	requestB := askToJoin(t, joiner, houseB)

	expectAs(t, firstOwner).
		POST("/houses/{h}/requests/{r}/approve", houseA, requestA).
		Expect().Status(http.StatusNoContent)

	// B's request is closed, so B's owner never sees it in their queue.
	expectAs(t, secondOwner).GET("/houses/{id}", houseB).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("pendingRequests").Array().IsEmpty()

	// And it cannot be approved after the fact.
	expectAs(t, secondOwner).
		POST("/houses/{h}/requests/{r}/approve", houseB, requestB).
		Expect().Status(http.StatusNotFound)
}

func TestApprovingALifterWhoJoinedElsewhereIsAConflict(t *testing.T) {
	_, firstOwner := secondLifter(t, "house-raced-a")
	_, secondOwner := secondLifter(t, "house-raced-b")
	_, joiner := secondLifter(t, "house-raced-joiner")
	houseA := foundHouse(t, firstOwner, "House Raced A", "HRCA")
	houseB := foundHouse(t, secondOwner, "House Raced B", "HRCB")

	requestB := askToJoin(t, joiner, houseB)

	// The lifter gets into A by some other route than the request above, which
	// leaves B's request open and pointing at somebody who now has a House. This
	// is the state SupersedeOtherOpenRequests exists to prevent; reaching it by
	// hand is what proves the approval path still refuses rather than relocating
	// them out of the House they chose.
	requestA := askToJoin(t, joiner, houseA)
	expectAs(t, firstOwner).
		POST("/houses/{h}/requests/{r}/approve", houseA, requestA).
		Expect().Status(http.StatusNoContent)
	if _, err := testPool.Exec(
		context.Background(),
		`UPDATE house_join_requests SET decided_at = NULL, outcome = NULL WHERE id = $1`,
		requestB,
	); err != nil {
		t.Fatalf("reopening request: %v", err)
	}

	expectAs(t, secondOwner).
		POST("/houses/{h}/requests/{r}/approve", houseB, requestB).
		Expect().Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("already_in_house")
}

func TestARequestFromAnotherHouseCannotBeDecided(t *testing.T) {
	_, firstOwner := secondLifter(t, "house-crossed-a")
	_, secondOwner := secondLifter(t, "house-crossed-b")
	_, joiner := secondLifter(t, "house-crossed-joiner")
	houseA := foundHouse(t, firstOwner, "House Crossed A", "HCRA")
	houseB := foundHouse(t, secondOwner, "House Crossed B", "HCRB")

	requestA := askToJoin(t, joiner, houseA)

	// B's owner holds A's request id. The House in the path is checked against
	// the one the request belongs to, so this is a 404 rather than one owner
	// admitting a lifter to somebody else's House.
	expectAs(t, secondOwner).
		POST("/houses/{h}/requests/{r}/approve", houseB, requestA).
		Expect().Status(http.StatusNotFound)
}

func TestLeavingTransfersOwnershipToTheLongestStandingMember(t *testing.T) {
	_, owner := secondLifter(t, "house-transfer-owner")
	elderID, elder := secondLifter(t, "house-transfer-elder")
	_, junior := secondLifter(t, "house-transfer-junior")
	houseID := foundHouse(t, owner, "House Transfer", "HTRF")

	for _, token := range []string{elder, junior} {
		req := askToJoin(t, token, houseID)
		expectAs(t, owner).
			POST("/houses/{h}/requests/{r}/approve", houseID, req).
			Expect().Status(http.StatusNoContent)
	}

	expectAs(t, owner).DELETE("/me/house").Expect().Status(http.StatusNoContent)

	// The elder now owns it, and can prove it by doing an owner-only thing.
	expectAs(t, elder).PATCH("/houses/{id}", houseID).
		WithJSON(map[string]any{"tagline": "ours now"}).
		Expect().Status(http.StatusOK)

	detail := expectAs(t, elder).GET("/houses/{id}", houseID).
		Expect().Status(http.StatusOK).JSON().Object()
	detail.Value("viewer").Object().Value("isOwner").Boolean().IsTrue()
	detail.Value("memberCount").Number().IsEqual(2)
	owning := detail.Value("members").Array().Value(0).Object()
	owning.Value("isOwner").Boolean().IsTrue()
	owning.Value("lifter").Object().Value("id").Number().IsEqual(elderID)

	// The lifter who left is free to found another.
	expectAs(t, owner).POST("/houses").
		WithJSON(map[string]any{"name": "House Transfer Again", "sigil": "HTRG"}).
		Expect().Status(http.StatusCreated)
	t.Cleanup(func() {
		_, _ = testPool.Exec(
			context.Background(), `DELETE FROM houses WHERE lower(sigil) = 'htrg'`,
		)
	})
}

// The last member out leaves the House STANDING. It was deleted once; it is not
// anymore, because a name, a sigil and a founding date belong to more lifters
// than whoever happened to leave last.
func TestTheLastMemberOutLeavesTheHouseStanding(t *testing.T) {
	_, owner := secondLifter(t, "house-last-out")
	_, asker := secondLifter(t, "house-last-out-asker")
	houseID := foundHouse(t, owner, "House Last Out", "HLOT")
	t.Cleanup(func() {
		_, _ = testPool.Exec(
			context.Background(), `DELETE FROM houses WHERE lower(sigil) = 'hlot'`,
		)
	})

	askToJoin(t, asker, houseID)

	expectAs(t, owner).DELETE("/me/house").Expect().Status(http.StatusNoContent)

	// Still there, still named, and now empty.
	detail := expectAs(t, owner).GET("/houses/{id}", houseID).
		Expect().Status(http.StatusOK).JSON().Object()
	detail.Value("name").String().IsEqual("House Last Out")
	detail.Value("memberCount").Number().IsEqual(0)
	detail.Value("members").Array().IsEmpty()

	// Leaving when in no House.
	expectAs(t, owner).DELETE("/me/house").Expect().Status(http.StatusNotFound)

	// The outstanding request was closed rather than cascaded away. Nobody is
	// left to answer it, and leaving it open would lock its own asker out of
	// claiming the House below — the page would offer them Withdraw, not ask.
	var open int
	if err := testPool.QueryRow(
		context.Background(),
		`SELECT COUNT(*) FROM house_join_requests
		 WHERE house_id = $1 AND decided_at IS NULL`, houseID,
	).Scan(&open); err != nil {
		t.Fatalf("counting open requests: %v", err)
	}
	if open != 0 {
		t.Fatalf("expected the House's open requests to be superseded, found %d", open)
	}

	// And the asker walks straight in, as its owner, rather than filing a request
	// that nobody could ever approve. 200-with-a-House, not 201-with-a-request.
	claimed := expectAs(t, asker).POST("/houses/{h}/requests", houseID).
		Expect().Status(http.StatusOK).JSON().Object()
	claimed.Value("id").Number().IsEqual(houseID)
	claimed.Value("memberCount").Number().IsEqual(1)
	viewer := claimed.Value("viewer").Object()
	viewer.Value("isMember").Boolean().IsTrue()
	viewer.Value("isOwner").Boolean().IsTrue()
}

// An empty House is claimed, not queued for. The ordinary ask files a row for an
// owner to answer, and an empty House has no owner to answer it.
func TestClaimingAnEmptyHouseSupersedesTheClaimantsOtherRequests(t *testing.T) {
	_, owner := secondLifter(t, "house-claim-owner")
	_, claimant := secondLifter(t, "house-claim-claimant")
	_, bystander := secondLifter(t, "house-claim-bystander")

	emptied := foundHouse(t, owner, "House Claim Empty", "HCLE")
	t.Cleanup(func() {
		_, _ = testPool.Exec(
			context.Background(), `DELETE FROM houses WHERE lower(sigil) = 'hcle'`,
		)
	})
	// A second, occupied House the claimant is also waiting on.
	occupied := foundHouse(t, bystander, "House Claim Occupied", "HCLO")

	elsewhere := askToJoin(t, claimant, occupied)

	// Empty the first House.
	expectAs(t, owner).DELETE("/me/house").Expect().Status(http.StatusNoContent)

	expectAs(t, claimant).POST("/houses/{h}/requests", emptied).
		Expect().Status(http.StatusOK)

	// Claiming is joining, so the queue they were sitting in elsewhere closes —
	// exactly as founding a House closes it. Left open, the bystander could
	// approve a lifter who already has a House.
	var outcome string
	if err := testPool.QueryRow(
		context.Background(),
		`SELECT outcome FROM house_join_requests WHERE id = $1`, elsewhere,
	).Scan(&outcome); err != nil {
		t.Fatalf("reading the other request: %v", err)
	}
	if outcome != "superseded" {
		t.Fatalf("expected the claimant's other request to be superseded, got %q", outcome)
	}

	// And a lifter who already has a House cannot claim an empty one.
	expectAs(t, claimant).POST("/houses/{h}/requests", occupied).
		Expect().Status(http.StatusConflict)
}

// Claiming an empty House takes the House's ROW LOCK before it decides.
//
// Without it, two lifters asking the same empty House at once would each read
// "no owner" under read committed and each walk in as owner: AddHouseMember's
// ON CONFLICT is on user_id, which forbids one LIFTER holding two Houses and has
// nothing to say about two lifters holding one.
//
// Asserted by holding the lock and showing the claim WAITS, rather than by
// firing concurrent claims and hoping they overlap. That was tried first and is
// worthless here: the window between the owner read and the commit is
// sub-millisecond, and the racing version passed against the unlocked code every
// time. This fails against it deterministically, which is the whole job of the
// test.
func TestClaimingAnEmptyHouseTakesTheHouseRowLock(t *testing.T) {
	_, owner := secondLifter(t, "house-race-owner")
	_, claimant := secondLifter(t, "house-race-claimant")
	houseID := foundHouse(t, owner, "House Race", "HRCE")
	t.Cleanup(func() {
		_, _ = testPool.Exec(
			context.Background(), `DELETE FROM houses WHERE lower(sigil) = 'hrce'`,
		)
	})

	expectAs(t, owner).DELETE("/me/house").Expect().Status(http.StatusNoContent)

	// Raw net/http rather than expectAs: httpexpect reports through *testing.T,
	// which is not safe to call from the goroutine below.
	claim := func(token string) int {
		req, err := http.NewRequest(
			http.MethodPost, fmt.Sprintf("%s/houses/%d/requests", baseURL, houseID), nil,
		)
		if err != nil {
			return 0
		}
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	ctx := context.Background()
	blocker, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the blocking transaction: %v", err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	// FOR KEY SHARE, and the weaker mode is the entire point. FOR UPDATE here
	// would prove nothing: AddHouseMember's insert carries a foreign key to
	// houses, so it takes a FOR KEY SHARE on this row by itself and would block
	// against FOR UPDATE whether or not the handler asks for a lock of its own.
	// FOR KEY SHARE is compatible with that implicit lock and conflicts only with
	// the handler's explicit FOR UPDATE, so this blocks if and only if LockHouse
	// ran.
	if _, err := blocker.Exec(
		ctx, `SELECT id FROM houses WHERE id = $1 FOR KEY SHARE`, houseID,
	); err != nil {
		t.Fatalf("locking the House row: %v", err)
	}

	done := make(chan int, 1)
	go func() { done <- claim(claimant) }()

	select {
	case code := <-done:
		t.Fatalf(
			"claim returned %d while the House row was locked — it is not taking the lock, "+
				"so two lifters can both claim an empty House", code,
		)
	case <-time.After(750 * time.Millisecond):
		// Blocked on the row, which is the point.
	}

	// Released: the claim proceeds and takes the House.
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatalf("releasing the lock: %v", err)
	}
	if code := <-done; code != http.StatusOK {
		t.Fatalf("expected the freed claim to take the House with 200, got %d", code)
	}

	// The invariant the lock is protecting.
	var owners int
	if err := testPool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM house_members WHERE house_id = $1 AND is_owner`, houseID,
	).Scan(&owners); err != nil {
		t.Fatalf("counting owners: %v", err)
	}
	if owners != 1 {
		t.Fatalf("expected exactly one owner, found %d", owners)
	}
}

// Owner-only writes against an empty House 404: there is no owner, so there is
// nobody they could be addressed to.
func TestAnEmptyHouseHasNoOwnerToActAsOne(t *testing.T) {
	_, owner := secondLifter(t, "house-empty-owner")
	_, stranger := secondLifter(t, "house-empty-stranger")
	houseID := foundHouse(t, owner, "House Empty Owner", "HEMO")
	t.Cleanup(func() {
		_, _ = testPool.Exec(
			context.Background(), `DELETE FROM houses WHERE lower(sigil) = 'hemo'`,
		)
	})

	expectAs(t, owner).DELETE("/me/house").Expect().Status(http.StatusNoContent)

	// The lifter who founded it has no standing over it once they walk out.
	expectAs(t, owner).PATCH("/houses/{id}", houseID).
		WithJSON(map[string]any{"tagline": "still mine"}).
		Expect().Status(http.StatusNotFound)
	expectAs(t, stranger).PATCH("/houses/{id}", houseID).
		WithJSON(map[string]any{"tagline": "mine now"}).
		Expect().Status(http.StatusNotFound)

	// But it reads fine, which is what makes it findable enough to claim.
	expectAs(t, stranger).GET("/houses/{id}", houseID).
		Expect().Status(http.StatusOK).JSON().Object().
		Value("viewer").Object().Value("isMember").Boolean().IsFalse()
}

func TestAnOwnerlessHouseFallsBackToItsLongestStandingMember(t *testing.T) {
	_, owner := secondLifter(t, "house-ownerless-owner")
	_, heir := secondLifter(t, "house-ownerless-heir")
	houseID := foundHouse(t, owner, "House Ownerless", "HOWN")

	req := askToJoin(t, heir, houseID)
	expectAs(t, owner).
		POST("/houses/{h}/requests/{r}/approve", houseID, req).
		Expect().Status(http.StatusNoContent)

	// The owner deletes their account. The cascade takes the membership row —
	// and its is_owner flag — with it, and no transfer runs, which is the one
	// state the schema cannot hold.
	if _, err := testPool.Exec(
		context.Background(),
		`DELETE FROM users WHERE lower(username) = 'house-ownerless-owner'`,
	); err != nil {
		t.Fatalf("deleting the owner: %v", err)
	}

	// Nobody is marked as owner...
	detail := expectAs(t, heir).GET("/houses/{id}", houseID).
		Expect().Status(http.StatusOK).JSON().Object()
	detail.Value("members").Array().Value(0).Object().
		Value("isOwner").Boolean().IsFalse()
	// ...but the House is still answerable, because the owner is a read.
	detail.Value("viewer").Object().Value("isOwner").Boolean().IsTrue()

	expectAs(t, heir).PATCH("/houses/{id}", houseID).
		WithJSON(map[string]any{"tagline": "inherited"}).
		Expect().Status(http.StatusOK)
}

func TestHousesDoNotGateAchievementsOrTheLeaderboard(t *testing.T) {
	// The guard on this feature's central decision. A House groups; it does not
	// gate — so a lifter in no House at all still reads everything they could
	// read before Houses existed. If this ever fails, the premise stated at the
	// top of lifters.go has been reversed rather than extended.
	_, owner := secondLifter(t, "house-openness-owner")
	_, outsider := secondLifter(t, "house-openness-outsider")
	foundHouse(t, owner, "House Openness", "HOPN")

	expectAs(t, outsider).GET("/achievements").
		Expect().Status(http.StatusOK).
		JSON().Object().ContainsKey("items")

	expectAs(t, outsider).GET("/leaderboard").
		Expect().Status(http.StatusOK).
		JSON().Object().ContainsKey("boards")

	// A bare array, unlike the two above — the roster predates the {items}
	// envelope the later collections use.
	expectAs(t, outsider).GET("/lifters").
		Expect().Status(http.StatusOK).
		JSON().Array().NotEmpty()
}

func TestAnUnknownHouseIsNotFound(t *testing.T) {
	_, token := secondLifter(t, "house-unknown")

	expectAs(t, token).GET("/houses/{id}", 2147483600).
		Expect().Status(http.StatusNotFound)
	expectAs(t, token).POST("/houses/{id}/requests", 2147483600).
		Expect().Status(http.StatusNotFound)
	expectAs(t, token).PATCH("/houses/{id}", 2147483600).
		WithJSON(map[string]any{"tagline": "nobody"}).
		Expect().Status(http.StatusNotFound)
	// Not a number at all.
	expectAs(t, token).GET("/houses/nope").Expect().Status(http.StatusNotFound)
}

func TestHousesRejectAnonymousCallers(t *testing.T) {
	expectAnon(t).GET("/houses").Expect().Status(http.StatusUnauthorized)
	expectAnon(t).POST("/houses").
		WithJSON(map[string]any{"name": "House Anon", "sigil": "HANO"}).
		Expect().Status(http.StatusUnauthorized)
	expectAnon(t).GET("/houses/1").Expect().Status(http.StatusUnauthorized)
	expectAnon(t).DELETE("/me/house").Expect().Status(http.StatusUnauthorized)
}

func TestHousesAreGatedUntilThePasswordChanges(t *testing.T) {
	const username = "house-gated"
	const password = "house-gated-pw"
	createAccount(t, username, password)
	token := signIn(t, username, password)

	// An account still carrying the password an admin chose for it reaches /me
	// and nothing else.
	expectAs(t, token).GET("/houses").
		Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("password_change_required")
	expectAs(t, token).DELETE("/me/house").
		Expect().Status(http.StatusForbidden)
}
