package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// The feed: what the other lifters on this install have been doing.
//
// Nothing here asserts the feed's total length or its absolute contents. Every
// test in this package shares one database, and the feed is the one endpoint that
// spans accounts — so it sees whatever history the rest of the suite happens to
// have left behind. The assertions are therefore about specific session ids:
// which of them appear, which do not, and in what order relative to each other.

// feedIDs reads the feed as the given caller and returns the session ids in the
// order they arrive.
func feedIDs(e *httpexpect.Expect, query ...any) []int {
	req := e.GET("/feed")
	for i := 0; i+1 < len(query); i += 2 {
		req = req.WithQuery(fmt.Sprint(query[i]), query[i+1])
	}
	items := req.Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array()

	out := make([]int, 0)
	for _, item := range items.Iter() {
		out = append(out, int(item.Object().Value("id").Number().Raw()))
	}
	return out
}

func contains(ids []int, id int) bool {
	for _, got := range ids {
		if got == id {
			return true
		}
	}
	return false
}

// indexOf is where an id sits in the feed, or -1. Used for relative ordering,
// which is the only ordering claim a shared database supports.
func indexOf(ids []int, id int) int {
	for i, got := range ids {
		if got == id {
			return i
		}
	}
	return -1
}

// loggedSessionOn starts a session for the given caller, logs its first set, and
// dates it. Returns the session id.
//
// The date is set through PATCH rather than by writing the row, because
// performed_on is what the feed orders by and a test that reached past the API to
// set it would not be exercising the same column the endpoint reads.
func loggedSessionOn(t *testing.T, e *httpexpect.Expect, dayID int, on string) int {
	t.Helper()
	session := startSession(t, e, dayID)
	id := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(e, id, setID, 5, true)
	e.PATCH(fmt.Sprintf("/sessions/%d", id)).
		WithJSON(map[string]any{"performedOn": on}).
		Expect().Status(http.StatusOK)
	return id
}

// ---- the gate ----

func TestFeedRejectsAnonymousCallers(t *testing.T) {
	expectAnon(t).GET("/feed").Expect().Status(http.StatusUnauthorized)
}

func TestFeedIsGatedUntilThePasswordChanges(t *testing.T) {
	createAccount(t, "feed-gated", "feed-gated-pw")
	token := signIn(t, "feed-gated", "feed-gated-pw")

	expectAs(t, token).GET("/feed").Expect().
		Status(http.StatusForbidden).
		JSON().Object().HasValue("code", "password_change_required")
}

// The feed reads, and only reads.
func TestFeedIsReadOnly(t *testing.T) {
	e := expect(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		e.Request(method, "/feed").
			WithJSON(map[string]any{}).
			Expect().Status(http.StatusMethodNotAllowed)
	}
}

// ---- whose sessions ----

// The defining behaviour: your own work is not in your feed, and theirs is.
//
// Excluded in SQL rather than left to the client, so that a page of ten is ten
// entries — see ListFeedSessions. A client-side filter would hand back however
// many of the ten happened not to be yours.
func TestFeedExcludesTheCallersOwnSessionsAndIncludesOthers(t *testing.T) {
	_, token := secondLifter(t, "feed-subject")
	theirs := expectAs(t, token)
	mine := expect(t)

	_, dayID := firstProgramAndDay(mine)
	myID := loggedSessionOn(t, mine, dayID, "2026-03-10")
	theirID := loggedSessionOn(t, theirs, dayID, "2026-03-11")

	asMe := feedIDs(mine)
	if contains(asMe, myID) {
		t.Errorf("my own session %d appears in my feed", myID)
	}
	if !contains(asMe, theirID) {
		t.Errorf("their session %d is missing from my feed", theirID)
	}

	// And symmetrically, which is what says the exclusion is about the caller
	// rather than about a particular account.
	asThem := feedIDs(theirs)
	if contains(asThem, theirID) {
		t.Errorf("their own session %d appears in their feed", theirID)
	}
	if !contains(asThem, myID) {
		t.Errorf("my session %d is missing from their feed", myID)
	}
}

// A session opened and abandoned without a logged rep is not something that
// happened. Same definition of a started session as GET /sessions, which is the
// point — the feed and the history page must not disagree about a row a lifter
// can open and read.
func TestFeedIgnoresASessionWithNoLoggedReps(t *testing.T) {
	_, token := secondLifter(t, "feed-empty-session")
	theirs := expectAs(t, token)

	_, dayID := firstProgramAndDay(theirs)
	abandoned := startSession(t, theirs, dayID)
	abandonedID := int(abandoned.Value("id").Number().Raw())

	if contains(feedIDs(expect(t)), abandonedID) {
		t.Errorf("session %d has no logged reps but appears in the feed", abandonedID)
	}

	// Log one rep and it becomes real.
	setID := int(abandoned.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(theirs, abandonedID, setID, 5, true)

	if !contains(feedIDs(expect(t)), abandonedID) {
		t.Errorf("session %d has a logged rep but is missing from the feed", abandonedID)
	}
}

// ---- order and paging ----

func TestFeedIsNewestFirst(t *testing.T) {
	_, token := secondLifter(t, "feed-order")
	theirs := expectAs(t, token)

	_, dayID := firstProgramAndDay(theirs)
	older := loggedSessionOn(t, theirs, dayID, "2026-02-02")
	newer := loggedSessionOn(t, theirs, dayID, "2026-02-09")

	ids := feedIDs(expect(t))
	oldAt, newAt := indexOf(ids, older), indexOf(ids, newer)
	if oldAt < 0 || newAt < 0 {
		t.Fatalf("both sessions should be in the feed, got positions %d and %d", oldAt, newAt)
	}
	if newAt > oldAt {
		t.Errorf("the newer session sits at %d, behind the older at %d", newAt, oldAt)
	}
}

func TestFeedPages(t *testing.T) {
	_, token := secondLifter(t, "feed-paging")
	theirs := expectAs(t, token)

	_, dayID := firstProgramAndDay(theirs)
	loggedSessionOn(t, theirs, dayID, "2026-01-05")
	loggedSessionOn(t, theirs, dayID, "2026-01-12")

	first := feedIDs(expect(t), "limit", 1)
	if len(first) != 1 {
		t.Fatalf("limit=1 returned %d items", len(first))
	}

	second := feedIDs(expect(t), "limit", 1, "offset", 1)
	if len(second) != 1 {
		t.Fatalf("limit=1&offset=1 returned %d items", len(second))
	}
	if first[0] == second[0] {
		t.Errorf("offset=1 returned the same session %d as offset=0", first[0])
	}

	// The window is echoed back, so a client paging can see what it was given
	// rather than assuming its own request was honoured.
	expect(t).GET("/feed").WithQuery("limit", 5).WithQuery("offset", 2).
		Expect().Status(http.StatusOK).JSON().Object().
		HasValue("limit", 5).HasValue("offset", 2)
}

// The paging bounds are shared with GET /sessions now that both read them
// through one helper. Asserted here too, because "shared" is the claim and an
// endpoint that quietly stopped using it would still pass the sessions tests.
func TestFeedValidatesItsPagingWindow(t *testing.T) {
	e := expect(t)
	for _, q := range []struct{ key, value string }{
		{"limit", "0"},
		{"limit", "101"},
		{"limit", "-1"},
		{"limit", "banana"},
		{"offset", "-1"},
		// Past int32. This used to wrap negative rather than being rejected, which
		// is the reason the parsing is one function.
		{"offset", "3000000000"},
	} {
		e.GET("/feed").WithQuery(q.key, q.value).
			Expect().Status(http.StatusBadRequest)
	}
}

// ---- the entry ----

func TestFeedEntryNamesTheLifterAndOmitsTheLifts(t *testing.T) {
	id, token := secondLifter(t, "feed-entry")
	theirs := expectAs(t, token)

	_, dayID := firstProgramAndDay(theirs)
	sessionID := loggedSessionOn(t, theirs, dayID, "2026-04-04")

	entry := expect(t).GET("/feed").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array().
		Find(func(_ int, value *httpexpect.Value) bool {
			return int(value.Object().Value("id").Number().Raw()) == sessionID
		}).Object()

	lifter := entry.Value("lifter").Object()
	lifter.HasValue("id", id)
	lifter.HasValue("username", "feed-entry")
	// The administrative columns stay behind here exactly as they do on the
	// roster — it is the same DTO, and this is what says so.
	lifter.NotContainsKey("isAdmin")
	lifter.NotContainsKey("mustChangePassword")
	lifter.NotContainsKey("barWeightLb")

	entry.HasValue("performedOn", "2026-04-04")
	entry.Value("programDayName").String().NotEmpty()
	entry.Value("volumeLb").Number().Gt(0)
	entry.Value("setCount").Number().Gt(0)

	// A card draws none of these, and fetching them would be a query per entry.
	entry.NotContainsKey("exercises")
	// Nor is this a fact about the lifter's whole record — the roster answers that.
	lifter.NotContainsKey("lastTrainedOn")
}

// An empty feed is [] and not null: a single-lifter install is the common case,
// and the surfaces that draw it branch on length.
func TestFeedIsAnArrayWhenThereIsNothingToShow(t *testing.T) {
	// Read from an account that has just been made, at an offset past anything
	// the suite could have left behind. The shape is what is under test, not the
	// contents.
	_, token := secondLifter(t, "feed-shape")

	expectAs(t, token).GET("/feed").WithQuery("offset", 10_000).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array().IsEmpty()
}
