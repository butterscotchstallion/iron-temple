package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// Notifications: what happened to you, addressed to you.
//
// Like the feed suite, nothing here asserts a total. Every test in this package
// shares one database and the primary account is the install's owner, so its
// panel accumulates whatever the rest of the suite's cross-account traffic left
// behind — and the generated-activity tests can add to it in bulk. The
// assertions are therefore about SPECIFIC rows: that a notification with this
// actor, on this session, of this kind, is present or absent.
//
// That is also why almost every test below acts as a freshly created lifter
// rather than as the primary account. A notification raised by a second lifter
// on a session owned by a third is a fact this suite can own outright.

// notificationsFor reads the panel as the given caller.
func notificationsFor(e *httpexpect.Expect, query ...any) *httpexpect.Array {
	req := e.GET("/notifications")
	for i := 0; i+1 < len(query); i += 2 {
		req = req.WithQuery(fmt.Sprint(query[i]), query[i+1])
	}
	return req.Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array()
}

// unreadCount is the badge the header draws.
func unreadCount(e *httpexpect.Expect) int {
	return int(e.GET("/notifications").Expect().Status(http.StatusOK).
		JSON().Object().Value("unreadCount").Number().Raw())
}

// matching counts the caller's notifications of a kind that name a session.
//
// Pages far enough back to find them: a notification raised a moment ago is at
// the head of the list, but the primary account's panel may already be deep, so
// callers that act as a fresh lifter get a short list and this stays cheap.
func matching(e *httpexpect.Expect, kind string, sessionID int) int {
	found := 0
	for _, item := range notificationsFor(e, "limit", 100).Iter() {
		obj := item.Object()
		if obj.Value("kind").String().Raw() != kind {
			continue
		}
		session := obj.Raw()["sessionId"]
		if session == nil {
			continue
		}
		if int(session.(float64)) == sessionID {
			found++
		}
	}
	return found
}

// TestApplauseNotifiesTheSessionsOwner is the behaviour the whole feature is
// for: something happened on your workout and you are told without going to
// look.
func TestApplauseNotifiesTheSessionsOwner(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-applause-owner")
	owner := expectAs(t, ownerToken)

	before := matching(owner, "reaction", sessionID)

	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "👏"}).
		Expect().Status(http.StatusNoContent)

	if got := matching(owner, "reaction", sessionID); got != before+1 {
		t.Fatalf("reaction notifications for session %d: got %d, want %d", sessionID, got, before+1)
	}

	// The row carries what a panel row draws, including which session it points
	// at — sessionOwnerId is how the client chooses between the two recap
	// routes, and here it is the caller's own.
	items := notificationsFor(owner, "limit", 100)
	var found *httpexpect.Object
	for _, item := range items.Iter() {
		obj := item.Object()
		raw := obj.Raw()
		if obj.Value("kind").String().Raw() == "reaction" &&
			raw["sessionId"] != nil && int(raw["sessionId"].(float64)) == sessionID {
			found = obj
			break
		}
	}
	if found == nil {
		t.Fatal("no reaction notification found for the session just applauded")
	}
	found.Value("emoji").String().IsEqual("👏")
	found.Value("actor").Object().Value("username").String().IsEqual("primary")
	found.ContainsKey("sessionOwnerId")
	found.ContainsKey("createdAt")
	// Unread, which is the state the badge counts.
	found.NotContainsKey("readAt")
}

// TestRepeatApplauseDoesNotNotifyTwice is why AddSessionReaction is :execrows.
// A double-tap is a no-op on the applause and must be one on the telling.
func TestRepeatApplauseDoesNotNotifyTwice(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-applause-repeat")
	owner := expectAs(t, ownerToken)

	before := matching(owner, "reaction", sessionID)

	for range 3 {
		expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
			WithJSON(map[string]any{"emoji": "🔥"}).
			Expect().Status(http.StatusNoContent)
	}

	if got := matching(owner, "reaction", sessionID); got != before+1 {
		t.Fatalf("three identical taps: got %d notifications, want %d", got, before+1)
	}
}

// TestWithdrawingApplauseWithdrawsTheNotification covers the asymmetry 0026
// records: a comment's notification goes by cascade, a reaction's has to be
// deleted by hand because session_reactions has no surrogate key to reference.
func TestWithdrawingApplauseWithdrawsTheNotification(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-applause-withdraw")
	owner := expectAs(t, ownerToken)

	before := matching(owner, "reaction", sessionID)

	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "🎉"}).
		Expect().Status(http.StatusNoContent)
	if got := matching(owner, "reaction", sessionID); got != before+1 {
		t.Fatalf("after applauding: got %d, want %d", got, before+1)
	}

	expect(t).DELETE(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithQuery("emoji", "🎉").
		Expect().Status(http.StatusNoContent)

	if got := matching(owner, "reaction", sessionID); got != before {
		t.Fatalf("after withdrawing: got %d, want %d", got, before)
	}
}

// TestCommentingNotifiesTheSessionsOwner is the comment half of the same rule.
func TestCommentingNotifiesTheSessionsOwner(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-comment-owner")
	owner := expectAs(t, ownerToken)

	before := matching(owner, "comment", sessionID)

	expect(t).POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "strong sets"}).
		Expect().Status(http.StatusCreated)

	if got := matching(owner, "comment", sessionID); got != before+1 {
		t.Fatalf("comment notifications: got %d, want %d", got, before+1)
	}

	items := notificationsFor(owner, "limit", 100)
	for _, item := range items.Iter() {
		obj := item.Object()
		raw := obj.Raw()
		if obj.Value("kind").String().Raw() == "comment" &&
			raw["sessionId"] != nil && int(raw["sessionId"].(float64)) == sessionID {
			// The body rides along in full, so a panel row can show what was
			// said rather than only that something was.
			obj.Value("commentBody").String().IsEqual("strong sets")
			return
		}
	}
	t.Fatal("no comment notification found for the session just commented on")
}

// TestCommentingOnYourOwnSessionNotifiesNobody.
//
// The API allows the comment — you may need to answer somebody — and the
// fan-out filters the author out of its own recipients.
func TestCommentingOnYourOwnSessionNotifiesNobody(t *testing.T) {
	_, token := secondLifter(t, "notify-comment-self")
	lifter := expectAs(t, token)
	_, dayID := firstProgramAndDay(lifter)
	session := startSession(t, lifter, dayID)
	sessionID := int(session.Value("id").Number().Raw())

	before := matching(lifter, "comment", sessionID)

	lifter.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "note to self"}).
		Expect().Status(http.StatusCreated)

	if got := matching(lifter, "comment", sessionID); got != before {
		t.Fatalf("commenting on your own session notified you: got %d, want %d", got, before)
	}
}

// TestSecondCommenterNotifiesTheOwnerAndTheFirstCommenter is the 'reply' kind,
// and it is the only way a notification reaches somebody about a session that
// is not theirs.
//
// It also covers the DISTINCT ON: the owner is in both halves of the fan-out's
// union once they have commented themselves, and must still be told exactly
// once about one comment.
func TestSecondCommenterNotifiesTheOwnerAndTheFirstCommenter(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-reply-owner")
	owner := expectAs(t, ownerToken)

	_, firstToken := secondLifter(t, "notify-reply-first")
	first := expectAs(t, firstToken)

	// The owner joins the conversation on their own session, which is what puts
	// them in both halves of the union.
	owner.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "felt good"}).
		Expect().Status(http.StatusCreated)

	first.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "nice work"}).
		Expect().Status(http.StatusCreated)

	ownerComments := matching(owner, "comment", sessionID)
	ownerReplies := matching(owner, "reply", sessionID)
	firstReplies := matching(first, "reply", sessionID)

	// A third lifter says something. The owner hears "commented", the earlier
	// commenter hears "reply".
	_, thirdToken := secondLifter(t, "notify-reply-third")
	expectAs(t, thirdToken).POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "how heavy?"}).
		Expect().Status(http.StatusCreated)

	if got := matching(owner, "comment", sessionID); got != ownerComments+1 {
		t.Fatalf("owner's 'comment' notifications: got %d, want %d", got, ownerComments+1)
	}
	// Exactly once: the owner is not ALSO told it was a reply, even though they
	// are among the session's commenters.
	if got := matching(owner, "reply", sessionID); got != ownerReplies {
		t.Fatalf("owner was told twice about one comment: 'reply' went %d -> %d", ownerReplies, got)
	}
	if got := matching(first, "reply", sessionID); got != firstReplies+1 {
		t.Fatalf("first commenter's 'reply' notifications: got %d, want %d", got, firstReplies+1)
	}
}

// TestDeletingACommentTakesItsNotification — by cascade, with no handler
// involved. See 0026.
func TestDeletingACommentTakesItsNotification(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-comment-deleted")
	owner := expectAs(t, ownerToken)

	before := matching(owner, "comment", sessionID)

	commentID := int(expect(t).POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "briefly said"}).
		Expect().Status(http.StatusCreated).
		JSON().Object().Value("id").Number().Raw())

	if got := matching(owner, "comment", sessionID); got != before+1 {
		t.Fatalf("after commenting: got %d, want %d", got, before+1)
	}

	expect(t).DELETE(fmt.Sprintf("/sessions/%d/comments/%d", sessionID, commentID)).
		Expect().Status(http.StatusNoContent)

	if got := matching(owner, "comment", sessionID); got != before {
		t.Fatalf("after deleting the comment: got %d, want %d", got, before)
	}
}

// TestANewAccountIsAnnouncedToEverybodyAlreadyHere.
func TestANewAccountIsAnnouncedToEverybodyAlreadyHere(t *testing.T) {
	_, existingToken := secondLifter(t, "notify-join-existing")
	existing := expectAs(t, existingToken)

	before := 0
	for _, item := range notificationsFor(existing, "limit", 100).Iter() {
		if item.Object().Value("kind").String().Raw() == "joined" {
			before++
		}
	}

	newID, _ := secondLifter(t, "notify-join-newcomer")

	after := 0
	announced := false
	for _, item := range notificationsFor(existing, "limit", 100).Iter() {
		obj := item.Object()
		if obj.Value("kind").String().Raw() != "joined" {
			continue
		}
		after++
		if int(obj.Value("actor").Object().Value("id").Number().Raw()) == int(newID) {
			announced = true
			// 'joined' is the kind whose subject is the actor themselves.
			obj.NotContainsKey("sessionId")
			obj.NotContainsKey("emoji")
		}
	}
	if after <= before {
		t.Fatalf("'joined' notifications: got %d, want more than %d", after, before)
	}
	if !announced {
		t.Fatalf("the new account (id %d) was not announced to an existing lifter", newID)
	}
}

// TestMarkingReadClearsTheBadgeAndKeepsTheList — the two buttons are different
// gestures, and this is the half that keeps the rows.
func TestMarkingReadClearsTheBadgeAndKeepsTheList(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-mark-read")
	owner := expectAs(t, ownerToken)

	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusNoContent)

	if unreadCount(owner) == 0 {
		t.Fatal("expected an unread notification after being applauded")
	}
	listed := len(notificationsFor(owner, "limit", 100).Iter())

	owner.POST("/notifications/read").Expect().Status(http.StatusNoContent)

	if got := unreadCount(owner); got != 0 {
		t.Fatalf("unread count after marking read: got %d, want 0", got)
	}
	if got := len(notificationsFor(owner, "limit", 100).Iter()); got != listed {
		t.Fatalf("marking read changed the list: got %d rows, want %d", got, listed)
	}
	// Now stamped, which is what the panel draws as "no longer new".
	notificationsFor(owner, "limit", 1).Value(0).Object().ContainsKey("readAt")

	// Idempotent.
	owner.POST("/notifications/read").Expect().Status(http.StatusNoContent)
	if got := unreadCount(owner); got != 0 {
		t.Fatalf("second mark-read: got %d, want 0", got)
	}
}

// TestClearingEmptiesTheListAndLeavesTheApplauseAlone — the other half. The
// rows go; the thing they described does not.
func TestClearingEmptiesTheListAndLeavesTheApplauseAlone(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-clear")
	owner := expectAs(t, ownerToken)

	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusNoContent)
	if len(notificationsFor(owner, "limit", 100).Iter()) == 0 {
		t.Fatal("expected a notification before clearing")
	}

	owner.DELETE("/notifications").Expect().Status(http.StatusNoContent)

	notificationsFor(owner, "limit", 100).IsEmpty()
	if got := unreadCount(owner); got != 0 {
		t.Fatalf("unread count after clearing: got %d, want 0", got)
	}

	// The applause itself is untouched — clearing is about the caller's panel,
	// not about the thing that happened.
	reactions(owner, sessionID).NotEmpty()

	// 204 on an empty panel, like withdrawing applause that was never given.
	owner.DELETE("/notifications").Expect().Status(http.StatusNoContent)
}

// TestNotificationsAreOnlyEverYourOwn. There is no id in the path and no way to
// ask for somebody else's, so this asserts the panels genuinely differ.
func TestNotificationsAreOnlyEverYourOwn(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-scoping-owner")
	owner := expectAs(t, ownerToken)

	_, bystanderToken := secondLifter(t, "notify-scoping-bystander")
	bystander := expectAs(t, bystanderToken)

	bystanderBefore := matching(bystander, "reaction", sessionID)

	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "🔥"}).
		Expect().Status(http.StatusNoContent)

	if matching(owner, "reaction", sessionID) == 0 {
		t.Fatal("the session's owner was not notified")
	}
	if got := matching(bystander, "reaction", sessionID); got != bystanderBefore {
		t.Fatalf("an uninvolved lifter was notified: got %d, want %d", got, bystanderBefore)
	}
}

// TestNotificationsPage covers the limit/offset contract — the same one the
// feed uses, including refusing a limit outside its bounds.
func TestNotificationsPage(t *testing.T) {
	e := expect(t)
	e.GET("/notifications").WithQuery("limit", 0).
		Expect().Status(http.StatusBadRequest)
	e.GET("/notifications").WithQuery("limit", 101).
		Expect().Status(http.StatusBadRequest)
	e.GET("/notifications").WithQuery("offset", -1).
		Expect().Status(http.StatusBadRequest)

	page := e.GET("/notifications").WithQuery("limit", 5).
		Expect().Status(http.StatusOK).JSON().Object()
	page.Value("limit").Number().IsEqual(5)
	page.Value("offset").Number().IsEqual(0)
	page.ContainsKey("unreadCount")
	if got := len(page.Value("items").Array().Iter()); got > 5 {
		t.Fatalf("limit 5 returned %d items", got)
	}
}

// TestNotificationsRejectAnonymous is the standard gate every authed endpoint
// in this package gets.
func TestNotificationsRejectAnonymous(t *testing.T) {
	anon := expectAnon(t)
	anon.GET("/notifications").Expect().Status(http.StatusUnauthorized)
	anon.POST("/notifications/read").Expect().Status(http.StatusUnauthorized)
	anon.POST("/notifications/1/read").Expect().Status(http.StatusUnauthorized)
	anon.DELETE("/notifications").Expect().Status(http.StatusUnauthorized)
}

// TestNotificationsAreGatedUntilThePasswordChanges is the other half of that
// gate: an account that has not replaced the password somebody else chose for
// it is refused everywhere except the two endpoints it needs to get out.
func TestNotificationsAreGatedUntilThePasswordChanges(t *testing.T) {
	const username = "notify-gated"
	const password = "notify-gated-pw"
	createAccount(t, username, password)
	token := signIn(t, username, password)
	gated := expectAs(t, token)

	gated.GET("/notifications").Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("password_change_required")
	gated.POST("/notifications/read").Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("password_change_required")
	gated.POST("/notifications/1/read").Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("password_change_required")
	gated.DELETE("/notifications").Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("password_change_required")
}

// ---- reading one ----

// findNotification returns the newest of the caller's notifications of a kind
// that names this session, failing the test if there is none.
//
// By (kind, session) rather than by position, for the reason the file header
// gives: the panel this reads may already hold whatever the rest of the suite
// left in it.
func findNotification(t *testing.T, e *httpexpect.Expect, kind string, sessionID int) *httpexpect.Object {
	t.Helper()
	for _, item := range notificationsFor(e, "limit", 100).Iter() {
		obj := item.Object()
		if obj.Value("kind").String().Raw() != kind {
			continue
		}
		session := obj.Raw()["sessionId"]
		if session != nil && int(session.(float64)) == sessionID {
			return obj
		}
	}
	t.Fatalf("no %s notification for session %d", kind, sessionID)
	return nil
}

func markRead(e *httpexpect.Expect, id int) {
	e.POST(fmt.Sprintf("/notifications/%d/read", id)).
		Expect().Status(http.StatusNoContent)
}

// Following one notification through to what it was about reads THAT one, and
// leaves the rows the lifter has not looked at still marked unread. Before this
// endpoint the only way to clear a badge was to mark everything read, which
// buried exactly the notifications worth keeping.
func TestMarkingOneNotificationReadLeavesTheOthers(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-read-one")
	owner := expectAs(t, ownerToken)
	_, otherToken := secondLifter(t, "notify-read-one-actor")

	// Two notifications on the same session, from two different lifters.
	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusNoContent)
	expectAs(t, otherToken).POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "good work"}).
		Expect().Status(http.StatusCreated)

	unreadBefore := unreadCount(owner)
	reaction := findNotification(t, owner, "reaction", sessionID)
	reaction.NotContainsKey("readAt")
	reactionID := int(reaction.Value("id").Number().Raw())

	markRead(owner, reactionID)

	// Exactly one row moved, not the panel.
	if got := unreadCount(owner); got != unreadBefore-1 {
		t.Fatalf("unread count %d after reading one, want %d", got, unreadBefore-1)
	}
	findNotification(t, owner, "reaction", sessionID).ContainsKey("readAt")
	findNotification(t, owner, "comment", sessionID).NotContainsKey("readAt")
}

// Idempotent, and readAt is when it was FIRST read — a second tap must not move
// the timestamp forward.
func TestMarkingOneNotificationReadTwiceKeepsTheFirstStamp(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-read-twice")
	owner := expectAs(t, ownerToken)

	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "🔥"}).
		Expect().Status(http.StatusNoContent)

	id := int(findNotification(t, owner, "reaction", sessionID).Value("id").Number().Raw())
	markRead(owner, id)
	first := findNotification(t, owner, "reaction", sessionID).Value("readAt").String().Raw()

	markRead(owner, id)
	if got := findNotification(t, owner, "reaction", sessionID).Value("readAt").String().Raw(); got != first {
		t.Fatalf("readAt moved from %q to %q on a second read", first, got)
	}
}

// Somebody else's notification is not the caller's to read. The UPDATE is scoped
// to the caller, so this is a no-op rather than a 403 — and deliberately not a
// 404, which would confirm the id exists.
func TestMarkingSomebodyElsesNotificationDoesNothing(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-read-foreign")
	owner := expectAs(t, ownerToken)

	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "👏"}).
		Expect().Status(http.StatusNoContent)

	id := int(findNotification(t, owner, "reaction", sessionID).Value("id").Number().Raw())
	before := unreadCount(owner)

	// The actor tries to read the notification they caused.
	expect(t).POST(fmt.Sprintf("/notifications/%d/read", id)).
		Expect().Status(http.StatusNoContent)

	if got := unreadCount(owner); got != before {
		t.Fatalf("owner's unread count moved from %d to %d on somebody else's read", before, got)
	}
	findNotification(t, owner, "reaction", sessionID).NotContainsKey("readAt")
}

// A notification for a row that never existed is the same 204. Nothing about
// this endpoint distinguishes "not yours" from "not there".
func TestMarkingAnUnknownNotificationReadIsAccepted(t *testing.T) {
	expect(t).POST("/notifications/99999999/read").
		Expect().Status(http.StatusNoContent)
}

// ---- the deep link ----

// commentId is what lets a client take the lifter to the sentence rather than
// to the page it is on.
func TestCommentNotificationCarriesItsCommentId(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-comment-id")
	owner := expectAs(t, ownerToken)

	posted := expect(t).POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "which comment was this"}).
		Expect().Status(http.StatusCreated).JSON().Object()
	commentID := int(posted.Value("id").Number().Raw())

	findNotification(t, owner, "comment", sessionID).
		HasValue("commentId", commentID)
}

// A reaction has no comment to point at, and says so by absence rather than by
// a zero a client would have to know to ignore.
func TestReactionNotificationCarriesNoCommentId(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-reaction-no-comment-id")
	owner := expectAs(t, ownerToken)

	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusNoContent)

	findNotification(t, owner, "reaction", sessionID).NotContainsKey("commentId")
}

// ---- the toggle ----

// Applaud, withdraw, applaud again. Each cycle is a genuine new reaction, and
// each one used to mint a fresh notification at the top of the owner's panel —
// which is how one lifter idly toggling a button becomes somebody else's unread
// count climbing.
func TestTogglingApplauseDoesNotStackNotifications(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-applause-toggle")
	owner := expectAs(t, ownerToken)
	path := fmt.Sprintf("/sessions/%d/reactions", sessionID)

	before := matching(owner, "reaction", sessionID)

	for range 5 {
		expect(t).POST(path).WithJSON(map[string]any{"emoji": "🎉"}).
			Expect().Status(http.StatusNoContent)
		expect(t).DELETE(path).WithQuery("emoji", "🎉").
			Expect().Status(http.StatusNoContent)
	}
	expect(t).POST(path).WithJSON(map[string]any{"emoji": "🎉"}).
		Expect().Status(http.StatusNoContent)

	// One row for one applause, however many times the button was pressed.
	if got := matching(owner, "reaction", sessionID); got != before+1 {
		t.Fatalf("after five toggles: got %d notifications, want %d", got, before+1)
	}
}

// Withdrawing applause retracts the notification only while it is still unseen.
// Once the owner has read it, the row stays: it records something that
// happened, and deleting it would be a notification vanishing between one poll
// and the next with nothing to explain it.
func TestWithdrawingApplauseKeepsANotificationAlreadyRead(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "notify-read-then-withdraw")
	owner := expectAs(t, ownerToken)
	path := fmt.Sprintf("/sessions/%d/reactions", sessionID)

	expect(t).POST(path).WithJSON(map[string]any{"emoji": "👏"}).
		Expect().Status(http.StatusNoContent)

	before := matching(owner, "reaction", sessionID)
	id := int(findNotification(t, owner, "reaction", sessionID).Value("id").Number().Raw())
	markRead(owner, id)

	expect(t).DELETE(path).WithQuery("emoji", "👏").
		Expect().Status(http.StatusNoContent)

	if got := matching(owner, "reaction", sessionID); got != before {
		t.Fatalf("a read notification went with the applause: got %d, want %d", got, before)
	}
	// The applause itself is gone — only the telling of it survives.
	reactions(expectAs(t, ownerToken), sessionID).IsEmpty()
}
