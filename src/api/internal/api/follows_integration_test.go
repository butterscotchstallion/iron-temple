package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// Following, which decides whose achievements reach whose panel.
//
// The fan-out itself is asserted in achievements_integration_test.go, where the
// crown is. What is here is the state: making it, unmaking it, refusing the two
// things it must refuse, and reporting it back on the two surfaces that draw a
// button from it.
//
// Every test acts as freshly created lifters rather than as the primary account,
// for the reason the notifications suite gives: the suite shares one database and
// the owner's account accumulates whatever the rest of it left behind. A follow
// between two accounts this test made is a fact it can own outright.

// following reports whether the caller follows this lifter, read off the profile.
func following(e *httpexpect.Expect, lifterID int32) bool {
	return e.GET(fmt.Sprintf("/lifters/%d", lifterID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("following").Boolean().Raw()
}

// followingInRoster is the same fact read off the roster, which is a different
// query and must agree.
func followingInRoster(t *testing.T, e *httpexpect.Expect, lifterID int32) bool {
	t.Helper()
	for _, item := range e.GET("/lifters").Expect().Status(http.StatusOK).JSON().Array().Iter() {
		obj := item.Object()
		if int32(obj.Value("id").Number().Raw()) == lifterID {
			return obj.Value("following").Boolean().Raw()
		}
	}
	t.Fatalf("lifter %d is not on the roster", lifterID)
	return false
}

func TestFollowingALifterAndStopping(t *testing.T) {
	targetID, _ := secondLifter(t, "follow-target")
	_, token := secondLifter(t, "follow-follower")
	me := expectAs(t, token)
	path := fmt.Sprintf("/me/following/%d", targetID)

	if following(me, targetID) {
		t.Fatal("a fresh account already follows somebody")
	}

	me.POST(path).Expect().Status(http.StatusNoContent)
	if !following(me, targetID) {
		t.Error("the profile does not report the follow that was just made")
	}

	me.DELETE(path).Expect().Status(http.StatusNoContent)
	if following(me, targetID) {
		t.Error("the profile still reports a follow that was removed")
	}
}

// THE ROSTER AND THE PROFILE MUST AGREE. They are two queries with the same
// correlated EXISTS, and users.sql says outright that a column added to one belongs
// in both — this is the assertion behind that sentence.
func TestTheRosterAndTheProfileAgreeAboutFollowing(t *testing.T) {
	targetID, _ := secondLifter(t, "follow-agree-target")
	_, token := secondLifter(t, "follow-agree-follower")
	me := expectAs(t, token)

	if followingInRoster(t, me, targetID) != following(me, targetID) {
		t.Fatal("roster and profile disagree before any follow")
	}

	me.POST(fmt.Sprintf("/me/following/%d", targetID)).
		Expect().Status(http.StatusNoContent)

	if !followingInRoster(t, me, targetID) {
		t.Error("the roster does not report the follow")
	}
	if followingInRoster(t, me, targetID) != following(me, targetID) {
		t.Error("roster and profile disagree after a follow")
	}
}

// A second press is a no-op and still a 204. The primary key settles it, and
// pressing a button twice is not a mistake worth reporting.
func TestFollowingTwiceIsANoOp(t *testing.T) {
	targetID, _ := secondLifter(t, "follow-twice-target")
	_, token := secondLifter(t, "follow-twice-follower")
	me := expectAs(t, token)
	path := fmt.Sprintf("/me/following/%d", targetID)

	me.POST(path).Expect().Status(http.StatusNoContent)
	me.POST(path).Expect().Status(http.StatusNoContent)

	if !following(me, targetID) {
		t.Error("a second press undid the first")
	}
}

// And unfollowing something that was never followed is the state the caller asked
// for, so it is a 204 rather than a 404.
func TestUnfollowingWhatWasNeverFollowedIsFine(t *testing.T) {
	targetID, _ := secondLifter(t, "unfollow-absent-target")
	_, token := secondLifter(t, "unfollow-absent-follower")

	expectAs(t, token).DELETE(fmt.Sprintf("/me/following/%d", targetID)).
		Expect().Status(http.StatusNoContent)
}

// Following yourself is refused rather than silently ignored: your own
// achievements already reach you, so the row could only mean what the default
// means, and a client that offered the button should hear about it.
func TestFollowingYourselfIsRefused(t *testing.T) {
	id, token := secondLifter(t, "follow-self")
	me := expectAs(t, token)
	path := fmt.Sprintf("/me/following/%d", id)

	me.POST(path).Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("own_account")
	me.DELETE(path).Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("own_account")
}

// An id that names nobody is a 404, not a foreign-key error reported as a 500.
func TestFollowingAnUnknownLifterIsNotFound(t *testing.T) {
	_, token := secondLifter(t, "follow-unknown")
	me := expectAs(t, token)

	me.POST("/me/following/99999999").Expect().Status(http.StatusNotFound)
	me.DELETE("/me/following/99999999").Expect().Status(http.StatusNotFound)
}

// A follow is one lifter's own, and another account's panel must not be reachable
// through it. Asserted by making a follow as one lifter and reading it as another.
func TestAFollowBelongsToTheCallerAlone(t *testing.T) {
	targetID, _ := secondLifter(t, "follow-scope-target")
	_, mineToken := secondLifter(t, "follow-scope-mine")
	_, theirsToken := secondLifter(t, "follow-scope-theirs")

	expectAs(t, mineToken).POST(fmt.Sprintf("/me/following/%d", targetID)).
		Expect().Status(http.StatusNoContent)

	if !following(expectAs(t, mineToken), targetID) {
		t.Fatal("the follower does not see their own follow")
	}
	if following(expectAs(t, theirsToken), targetID) {
		t.Error("one lifter's follow showed up as another's")
	}
}

// The roster still lists everybody, followed or not. This is the assertion behind
// the claim that a follow is about delivery and not visibility — lifters.go's
// premise is intact and this is what would catch it quietly changing.
func TestFollowingHidesNobodyFromTheRoster(t *testing.T) {
	targetID, _ := secondLifter(t, "follow-hides-nobody")
	_, token := secondLifter(t, "follow-hides-reader")
	me := expectAs(t, token)

	before := me.GET("/lifters").Expect().Status(http.StatusOK).JSON().Array().Length().Raw()
	me.POST(fmt.Sprintf("/me/following/%d", targetID)).
		Expect().Status(http.StatusNoContent)
	after := me.GET("/lifters").Expect().Status(http.StatusOK).JSON().Array().Length().Raw()

	if before != after {
		t.Errorf("the roster changed length across a follow: %v then %v", before, after)
	}
	// And the unfollowed accounts are all still there.
	if !followingInRoster(t, me, targetID) {
		t.Error("the followed lifter left the roster")
	}
}

func TestFollowingRejectsAnonymousCallers(t *testing.T) {
	anon := expectAnon(t)
	anon.POST("/me/following/1").Expect().Status(http.StatusUnauthorized)
	anon.DELETE("/me/following/1").Expect().Status(http.StatusUnauthorized)
}

// The other half of that gate: an account still carrying the password somebody else
// chose for it is refused everywhere except the two endpoints it needs to escape.
func TestFollowingIsGatedUntilThePasswordChanges(t *testing.T) {
	const username = "follow-gated"
	const password = "follow-gated-pw"
	createAccount(t, username, password)
	gated := expectAs(t, signIn(t, username, password))

	gated.POST("/me/following/1").Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("password_change_required")
	gated.DELETE("/me/following/1").Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("password_change_required")
}
