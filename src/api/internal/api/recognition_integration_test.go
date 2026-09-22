package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// Applause and conversation.
//
// The first test below is the one that matters. Everywhere else in this API a
// session is resolved by (id, user_id), which answers 404 for anybody but its
// owner — so the obvious implementation of this feature would refuse every session
// it exists to act on. That failure would look like the feature not working rather
// than like a scoping mistake, and it is the reason SessionExists exists.

// otherLiftersSession returns a session belonging to somebody who is not the
// primary account, with a rep logged against it.
func otherLiftersSession(t *testing.T, username string) (sessionID int, ownerToken string) {
	t.Helper()
	_, token := secondLifter(t, username)
	theirs := expectAs(t, token)
	_, dayID := firstProgramAndDay(theirs)
	session := startSession(t, theirs, dayID)
	sessionID = int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(theirs, sessionID, setID, 5, true)
	return sessionID, token
}

func reactions(e *httpexpect.Expect, sessionID int) *httpexpect.Array {
	return e.GET(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		Expect().Status(http.StatusOK).JSON().Array()
}

func comments(e *httpexpect.Expect, sessionID int) *httpexpect.Array {
	return e.GET(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		Expect().Status(http.StatusOK).JSON().Array()
}

// ---- the trap ----

// Reacting to a session you do not own is the whole point, so it gets its own
// test. GetSession would have made this a 404.
func TestReactionWorksOnSomebodyElsesSession(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "applause-target")
	e := expect(t)

	e.POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusNoContent)

	list := reactions(e, sessionID)
	list.Length().IsEqual(1)
	first := list.Value(0).Object()
	first.HasValue("emoji", "💪")
	first.HasValue("count", 1)
	first.HasValue("mine", true)
}

// And the owner sees it as applause from somebody else, not as their own.
func TestReactionMineIsPerReader(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "applause-mine")

	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "🔥"}).
		Expect().Status(http.StatusNoContent)

	owner := reactions(expectAs(t, ownerToken), sessionID).Value(0).Object()
	owner.HasValue("count", 1)
	owner.HasValue("mine", false)
}

// ---- reactions ----

// A double-tap is one reaction, settled by the primary key. It answers 204 like
// the first tap, because it is not a mistake the lifter should be told about.
func TestReactionIsIdempotent(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "applause-twice")
	e := expect(t)

	for range 3 {
		e.POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
			WithJSON(map[string]any{"emoji": "👏"}).
			Expect().Status(http.StatusNoContent)
	}

	reactions(e, sessionID).Value(0).Object().HasValue("count", 1)
}

// Applauding yourself is not a thing to record.
func TestReactionRefusedOnYourOwnSession(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	mine := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(e, mine, setID, 5, true)

	e.POST(fmt.Sprintf("/sessions/%d/reactions", mine)).
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusForbidden).
		JSON().Object().HasValue("code", "own_session")

	// Reading your own is fine — that is how you see who applauded.
	reactions(e, mine).IsEmpty()
}

func TestReactionEmojiMustBeOneOnOffer(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "applause-allowlist")
	e := expect(t)

	for _, emoji := range []string{"🦆", "", "not an emoji", "💪💪"} {
		e.POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
			WithJSON(map[string]any{"emoji": emoji}).
			Expect().Status(http.StatusBadRequest)
	}
	// The DELETE validates the same set, so a junk value cannot reach the query.
	e.DELETE(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithQuery("emoji", "🦆").
		Expect().Status(http.StatusBadRequest)

	reactions(e, sessionID).IsEmpty()
}

// Withdrawing is 204 whether or not there was anything there: the caller asked for
// a state, and it holds either way.
func TestReactionWithdrawalIsIdempotent(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "applause-undo")
	e := expect(t)
	path := fmt.Sprintf("/sessions/%d/reactions", sessionID)

	e.POST(path).WithJSON(map[string]any{"emoji": "🎉"}).
		Expect().Status(http.StatusNoContent)
	e.DELETE(path).WithQuery("emoji", "🎉").Expect().Status(http.StatusNoContent)
	// Again, with nothing to remove.
	e.DELETE(path).WithQuery("emoji", "🎉").Expect().Status(http.StatusNoContent)

	reactions(e, sessionID).IsEmpty()
}

// Different emoji are different reactions from the same lifter, and they are
// grouped rather than listed one per row.
func TestReactionsAreGroupedByEmoji(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "applause-group")
	_, thirdToken := secondLifter(t, "applause-third")

	for _, e := range []*httpexpect.Expect{expect(t), expectAs(t, thirdToken)} {
		e.POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
			WithJSON(map[string]any{"emoji": "💪"}).
			Expect().Status(http.StatusNoContent)
	}
	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "🔥"}).
		Expect().Status(http.StatusNoContent)

	list := reactions(expectAs(t, ownerToken), sessionID)
	list.Length().IsEqual(2)
	// Strongest first: two 💪 outrank one 🔥.
	list.Value(0).Object().HasValue("emoji", "💪").HasValue("count", 2)
	list.Value(1).Object().HasValue("emoji", "🔥").HasValue("count", 1)
}

func TestReactionOnAnUnknownSessionIsNotFound(t *testing.T) {
	e := expect(t)
	e.POST("/sessions/99999999/reactions").
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusNotFound)
	e.GET("/sessions/99999999/reactions").Expect().Status(http.StatusNotFound)
}

// ---- comments ----

func TestCommentOnSomebodyElsesSession(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "comment-target")
	e := expect(t)

	posted := e.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "  strong session  "}).
		Expect().Status(http.StatusCreated).JSON().Object()

	// Trimmed on the way in.
	posted.HasValue("body", "strong session")
	posted.HasValue("sessionId", sessionID)
	author := posted.Value("author").Object()
	author.HasValue("username", primaryUsername)
	// Same lifterDTO as the roster and the feed, so the administrative columns
	// stay behind here too.
	author.NotContainsKey("isAdmin")
	author.NotContainsKey("mustChangePassword")

	comments(e, sessionID).Length().IsEqual(1)
}

// Unlike a reaction: answering somebody on your own workout is the obvious thing
// to want.
func TestCommentIsAllowedOnYourOwnSession(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	mine := int(session.Value("id").Number().Raw())

	e.POST(fmt.Sprintf("/sessions/%d/comments", mine)).
		WithJSON(map[string]any{"body": "felt easier than it looked"}).
		Expect().Status(http.StatusCreated)
}

func TestCommentBodyIsValidated(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "comment-validation")
	e := expect(t)

	for _, body := range []string{"", "   ", "\n\t "} {
		e.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
			WithJSON(map[string]any{"body": body}).
			Expect().Status(http.StatusBadRequest)
	}

	e.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": strings.Repeat("a", 257)}).
		Expect().Status(http.StatusBadRequest)

	// Exactly at the cap is fine.
	e.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": strings.Repeat("a", 256)}).
		Expect().Status(http.StatusCreated)

	// And the cap counts runes, not bytes — 256 emoji is 1 KB and is a valid
	// comment. This is the assertion that fails if the limit ever moves onto the
	// column as a VARCHAR(256).
	e.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": strings.Repeat("🏋", 256)}).
		Expect().Status(http.StatusCreated)
}

func TestCommentsAreOldestFirst(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "comment-order")
	e := expect(t)

	for _, body := range []string{"first", "second", "third"} {
		e.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
			WithJSON(map[string]any{"body": body}).
			Expect().Status(http.StatusCreated)
	}

	list := comments(e, sessionID)
	list.Value(0).Object().HasValue("body", "first")
	list.Value(2).Object().HasValue("body", "third")
}

// ---- deletion ----

func TestCommentAuthorCanRemoveTheirOwn(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "comment-own-delete")
	_, token := secondLifter(t, "comment-author")
	author := expectAs(t, token)

	id := int(author.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "nice work"}).
		Expect().Status(http.StatusCreated).
		JSON().Object().Value("id").Number().Raw())

	author.DELETE(fmt.Sprintf("/sessions/%d/comments/%d", sessionID, id)).
		Expect().Status(http.StatusNoContent)

	comments(author, sessionID).IsEmpty()
}

// A third party cannot. Notably the session's OWNER cannot either — it is their
// workout, but it is not their comment, and moderation belongs to the install
// rather than to whoever is being talked about.
func TestCommentCannotBeRemovedByAnybodyElse(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "comment-guard")
	_, thirdToken := secondLifter(t, "comment-third")

	id := int(expectAs(t, thirdToken).POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "mine to delete"}).
		Expect().Status(http.StatusCreated).
		JSON().Object().Value("id").Number().Raw())

	expectAs(t, ownerToken).
		DELETE(fmt.Sprintf("/sessions/%d/comments/%d", sessionID, id)).
		Expect().Status(http.StatusForbidden).
		JSON().Object().HasValue("code", "not_your_comment")

	comments(expectAs(t, thirdToken), sessionID).Length().IsEqual(1)
}

// The install's owner may remove any. This is the only moderation the app has.
func TestAdminCanRemoveAnybodysComment(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "comment-moderation")
	_, thirdToken := secondLifter(t, "comment-moderated")

	id := int(expectAs(t, thirdToken).POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "something to moderate"}).
		Expect().Status(http.StatusCreated).
		JSON().Object().Value("id").Number().Raw())

	// expect(t) is the account that claimed the install.
	expect(t).DELETE(fmt.Sprintf("/sessions/%d/comments/%d", sessionID, id)).
		Expect().Status(http.StatusNoContent)
}

// A comment id under the wrong session is a 404. Without this check the session
// in the path would be decorative and one comment could be deleted through
// another's URL.
func TestCommentCannotBeRemovedThroughAnotherSessionsURL(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "comment-wrong-url")
	otherID, _ := otherLiftersSession(t, "comment-wrong-url-2")
	e := expect(t)

	id := int(e.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "attached to one session"}).
		Expect().Status(http.StatusCreated).
		JSON().Object().Value("id").Number().Raw())

	e.DELETE(fmt.Sprintf("/sessions/%d/comments/%d", otherID, id)).
		Expect().Status(http.StatusNotFound)

	// Still there.
	comments(e, sessionID).Length().IsEqual(1)
}

// ---- the gate, and the feed ----

func TestRecognitionRejectsAnonymousCallers(t *testing.T) {
	e := expectAnon(t)
	e.GET("/sessions/1/reactions").Expect().Status(http.StatusUnauthorized)
	e.POST("/sessions/1/reactions").
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusUnauthorized)
	e.GET("/sessions/1/comments").Expect().Status(http.StatusUnauthorized)
	e.POST("/sessions/1/comments").
		WithJSON(map[string]any{"body": "hello"}).
		Expect().Status(http.StatusUnauthorized)
	e.DELETE("/sessions/1/comments/1").Expect().Status(http.StatusUnauthorized)
}

// The feed carries the counts, so a row can show that something landed well.
func TestFeedCarriesRecognitionCounts(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "feed-counts")
	e := expect(t)

	entry := func() *httpexpect.Object {
		return e.GET("/feed").Expect().Status(http.StatusOK).
			JSON().Object().Value("items").Array().
			Find(func(_ int, value *httpexpect.Value) bool {
				return int(value.Object().Value("id").Number().Raw()) == sessionID
			}).Object()
	}

	before := entry()
	before.HasValue("reactionCount", 0)
	before.HasValue("commentCount", 0)

	e.POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusNoContent)
	e.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "big lift"}).
		Expect().Status(http.StatusCreated)

	after := entry()
	after.HasValue("reactionCount", 1)
	after.HasValue("commentCount", 1)
	// The volume is unchanged by any of this — the counts are two independent
	// subqueries and must not have fanned the aggregates out.
	after.Value("volumeLb").Number().IsEqual(before.Value("volumeLb").Number().Raw())
	after.Value("setCount").Number().IsEqual(before.Value("setCount").Number().Raw())
}

// Deleting a session takes its applause and its conversation with it. A reaction
// is about a workout and means nothing without one.
//
// Asserted against the tables rather than by reading a 404 off the API: once the
// session is gone every endpoint answers 404 whether the rows went with it or were
// left orphaned, so the HTTP surface cannot tell the two apart. Only the database
// can.
func TestRecognitionCascadesWhenASessionGoes(t *testing.T) {
	_, token := secondLifter(t, "cascade-subject")
	theirs := expectAs(t, token)
	_, dayID := firstProgramAndDay(theirs)

	// Deliberately not startSession: that registers a cleanup which deletes the
	// session and asserts a 204, and this test deletes it itself — the cleanup
	// would then fail on the 404 its own success caused.
	created := theirs.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated).JSON().Object()
	sessionID := int(created.Value("id").Number().Raw())
	setID := int(created.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(theirs, sessionID, setID, 5, true)

	e := expect(t)
	e.POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusNoContent)
	e.POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "gone in a moment"}).
		Expect().Status(http.StatusCreated)

	theirs.DELETE(fmt.Sprintf("/sessions/%d", sessionID)).
		Expect().Status(http.StatusNoContent)

	for _, table := range []string{"session_reactions", "session_comments"} {
		var left int
		err := testPool.QueryRow(context.Background(),
			// #nosec G201 -- table is from the literal slice above, not from input.
			fmt.Sprintf("SELECT count(*) FROM %s WHERE session_id = $1", table),
			sessionID).Scan(&left)
		if err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if left != 0 {
			t.Errorf("%s left %d orphaned rows after the session was deleted", table, left)
		}
	}
}
