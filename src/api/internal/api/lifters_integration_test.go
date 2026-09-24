package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// One lifter reading another.
//
// The assertions that matter here are the negative ones. /lifters is the first
// place in the app where a user id arrives from the URL rather than from the
// session cookie, so the interesting questions are not "does the roster render"
// but "can this subtree write", "does a mismatched pair leak a third lifter's
// session", and "do the administrative columns stay behind". Those get the most
// coverage below; the happy paths are mostly there to prove the reports are the
// SAME reports, not new ones.

// lifterRow finds one account in the roster by username, failing the test if it
// is absent. By username and not by position: the suite shares one database and
// other tests create accounts of their own, so nothing may depend on how many
// rows the roster has or what order they arrive in.
func lifterRow(e *httpexpect.Expect, username string) *httpexpect.Object {
	return e.GET("/lifters").Expect().Status(http.StatusOK).
		JSON().Array().
		Find(func(_ int, value *httpexpect.Value) bool {
			return value.Object().Value("username").String().Raw() == username
		}).Object()
}

// ---- the subtree is read-only ----

// The whole point of the /lifters subtree is that it only ever reads. Every
// route is asserted against every writing verb, because the guarantee is
// structural — there is no write handler to forget an ownership check in — and a
// structural guarantee is worth a test that would notice one being added.
func TestLifterRoutesAreReadOnly(t *testing.T) {
	id, token := secondLifter(t, "read-only-subject")
	theirs := expectAs(t, token)
	e := expect(t)

	// A real session of theirs, so the recap route below is a live URL rather
	// than one that could answer 405 for want of ever matching anything.
	_, dayID := firstProgramAndDay(theirs)
	session := startSession(t, theirs, dayID)
	sessionID := int(session.Value("id").Number().Raw())

	paths := []string{
		"/lifters",
		fmt.Sprintf("/lifters/%d", id),
		fmt.Sprintf("/lifters/%d/racked", id),
		fmt.Sprintf("/lifters/%d/sessions", id),
		fmt.Sprintf("/lifters/%d/sessions/%d/recap", id, sessionID),
	}
	for _, path := range paths {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			e.Request(method, path).
				WithJSON(map[string]any{"displayName": "hijacked"}).
				Expect().
				Status(http.StatusMethodNotAllowed)
		}
	}
}

// ---- the gate ----

func TestLifterRoutesRejectAnonymousCallers(t *testing.T) {
	e := expectAnon(t)
	e.GET("/lifters").Expect().Status(http.StatusUnauthorized)
	e.GET("/lifters/1").Expect().Status(http.StatusUnauthorized)
	e.GET("/lifters/1/racked").Expect().Status(http.StatusUnauthorized)
	e.GET("/lifters/1/sessions").Expect().Status(http.StatusUnauthorized)
	e.GET("/lifters/1/sessions/1/recap").Expect().Status(http.StatusUnauthorized)
}

// ---- the roster ----

// The roster must not carry the two columns ListUsers has and ListLifters
// deliberately drops. mustChangePassword is the sharp one: it says a one-time
// credential is still live, which is the owner's business and nobody else's.
func TestLifterRosterOmitsAdministrativeColumns(t *testing.T) {
	secondLifter(t, "no-admin-columns")

	row := lifterRow(expect(t), "no-admin-columns")
	row.NotContainsKey("isAdmin")
	row.NotContainsKey("mustChangePassword")
	// Nor the gym, which decides this lifter's weights and must never be read
	// as a basis for anybody else's plate maths.
	row.NotContainsKey("plates")
	row.NotContainsKey("barWeightLb")
	row.NotContainsKey("dumbbellStepLb")
	row.NotContainsKey("machineStepLb")
	row.NotContainsKey("cableStepLb")
	row.NotContainsKey("bandStepLb")
	// Nor whether they have got round to checking it, which is nobody else's
	// business and is not a fact about this lifter's training.
	row.NotContainsKey("equipmentConfirmedAt")

	row.Value("displayName").String().NotEmpty()
	row.Value("hasAvatar").Boolean().IsFalse()
}

// An ordinary lifter sees the roster, including the owner. There is no
// visibility setting to respect — admission to the install is the consent — so
// this asserts the decision rather than merely exercising the endpoint.
func TestLifterRosterIsVisibleToOrdinaryAccounts(t *testing.T) {
	_, token := secondLifter(t, "ordinary-reader")

	e := expectAs(t, token)
	lifterRow(e, "ordinary-reader").Value("username").String().IsEqual("ordinary-reader")
	lifterRow(e, primaryUsername).Value("username").String().IsEqual(primaryUsername)
}

// lastTrainedOn is absent until the account logs a rep, and dated once it has.
// Absent rather than zeroed, so a client never has to tell a real date from a
// stand-in one.
func TestLifterRosterReportsWhenTheyLastTrained(t *testing.T) {
	_, token := secondLifter(t, "last-trained")
	theirs := expectAs(t, token)

	lifterRow(expect(t), "last-trained").NotContainsKey("lastTrainedOn")

	_, dayID := firstProgramAndDay(theirs)
	session := startSession(t, theirs, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(theirs, sessionID, setID, 5, true)

	lifterRow(expect(t), "last-trained").Value("lastTrainedOn").String().NotEmpty()
}

// A session opened and walked away from without a single logged rep is not a day
// somebody trained. The roster uses the same definition as the history page, and
// this is the test that keeps the two honest.
func TestLifterRosterIgnoresASessionWithNoLoggedReps(t *testing.T) {
	_, token := secondLifter(t, "opened-nothing")
	theirs := expectAs(t, token)

	_, dayID := firstProgramAndDay(theirs)
	startSession(t, theirs, dayID)

	lifterRow(expect(t), "opened-nothing").NotContainsKey("lastTrainedOn")
}

// ---- the profile ----

func TestLifterProfileUnknownLifterIsNotFound(t *testing.T) {
	e := expect(t)
	e.GET("/lifters/99999999").Expect().Status(http.StatusNotFound)
	// A non-numeric id is the same answer, not a 400: the resource named by the
	// URL does not exist either way.
	e.GET("/lifters/not-a-number").Expect().Status(http.StatusNotFound)
	e.GET("/lifters/0").Expect().Status(http.StatusNotFound)
	e.GET("/lifters/-1").Expect().Status(http.StatusNotFound)
}

func TestLifterProfileCarriesLifetimeTotals(t *testing.T) {
	id, token := secondLifter(t, "lifetime-totals")
	theirs := expectAs(t, token)

	// A fresh account has lifted nothing, and says so with zeroes rather than
	// with absent fields — "no history" is a number here, not a gap.
	fresh := expect(t).GET(fmt.Sprintf("/lifters/%d", id)).Expect().
		Status(http.StatusOK).JSON().Object()
	fresh.HasValue("sessionCount", 0)
	fresh.HasValue("lifetimeVolumeLb", 0)

	_, dayID := firstProgramAndDay(theirs)
	session := startSession(t, theirs, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	first := session.Value("sets").Array().Value(0).Object()
	setID := int(first.Value("id").Number().Raw())
	weight := first.Value("weightLb").Number().Raw()
	logSet(theirs, sessionID, setID, 5, true)

	after := expect(t).GET(fmt.Sprintf("/lifters/%d", id)).Expect().
		Status(http.StatusOK).JSON().Object()
	after.HasValue("sessionCount", 1)
	after.Value("lifetimeVolumeLb").Number().IsEqual(5 * weight)
}

// The profile the owner reads and the totals the lifter reads on their own
// history page are the same two numbers. They come from one query (SessionTotals
// with no program filter), and this is what says so.
func TestLifterProfileTotalsMatchTheirOwnHistory(t *testing.T) {
	id, token := secondLifter(t, "totals-agree")
	theirs := expectAs(t, token)

	_, dayID := firstProgramAndDay(theirs)
	session := startSession(t, theirs, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(theirs, sessionID, setID, 3, false)

	own := theirs.GET("/sessions").Expect().Status(http.StatusOK).JSON().Object()
	profile := expect(t).GET(fmt.Sprintf("/lifters/%d", id)).Expect().
		Status(http.StatusOK).JSON().Object()

	profile.Value("sessionCount").Number().
		IsEqual(own.Value("total").Number().Raw())
	profile.Value("lifetimeVolumeLb").Number().
		IsEqual(own.Value("totalVolumeLb").Number().Raw())
}

// ---- racked ----

// The report one lifter reads about another is byte-for-byte the report that
// lifter reads about themselves.
//
// This is the assertion the whole design rests on. buildRacked takes the user as
// a parameter, so there is one implementation and no second opinion — and if
// somebody ever "optimises" the social path into its own query, this is the test
// that fails.
func TestLifterRackedIsTheSameReportTheyReadThemselves(t *testing.T) {
	id, token := secondLifter(t, "racked-agrees")
	theirs := expectAs(t, token)

	_, dayID := firstProgramAndDay(theirs)
	session := startSession(t, theirs, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(theirs, sessionID, setID, 5, true)

	own := theirs.GET("/racked").Expect().Status(http.StatusOK).JSON().Raw()

	expect(t).GET(fmt.Sprintf("/lifters/%d/racked", id)).Expect().
		Status(http.StatusOK).JSON().IsEqual(own)
}

// The period parameters are the ones /racked takes, parsed by the same code.
func TestLifterRackedHonoursThePeriodWindow(t *testing.T) {
	id, _ := secondLifter(t, "racked-window")
	e := expect(t)

	for _, period := range []string{"week", "month", "year"} {
		e.GET(fmt.Sprintf("/lifters/%d/racked", id)).
			WithQuery("period", period).
			Expect().Status(http.StatusOK).
			JSON().Object().Value("period").Object().HasValue("kind", period)
	}

	e.GET(fmt.Sprintf("/lifters/%d/racked", id)).
		WithQuery("period", "fortnight").
		Expect().Status(http.StatusBadRequest)
	e.GET(fmt.Sprintf("/lifters/%d/racked", id)).
		WithQuery("on", "last-tuesday").
		Expect().Status(http.StatusBadRequest)
}

// An unknown lifter is a 404 and NOT an empty report.
//
// The queries behind a Racked report match no rows for an id that names nobody,
// and reduce to a perfectly well-formed report of zeroes. Serving that would tell
// the caller a lifter who does not exist trained nothing, rather than that they
// asked about nobody — which is why lifterFromPath exists at all.
func TestLifterRackedUnknownLifterIsNotAnEmptyReport(t *testing.T) {
	expect(t).GET("/lifters/99999999/racked").Expect().Status(http.StatusNotFound)
}

// ---- session recaps ----

// The recap one lifter reads about another's session is the recap its owner
// reads. Same reasoning as the Racked report above.
func TestLifterSessionRecapIsTheSameRecapTheOwnerReads(t *testing.T) {
	id, token := secondLifter(t, "recap-agrees")
	theirs := expectAs(t, token)

	_, dayID := firstProgramAndDay(theirs)
	session := startSession(t, theirs, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(theirs, sessionID, setID, 5, true)

	own := theirs.GET(fmt.Sprintf("/sessions/%d/recap", sessionID)).
		Expect().Status(http.StatusOK).JSON().Raw()

	expect(t).GET(fmt.Sprintf("/lifters/%d/sessions/%d/recap", id, sessionID)).
		Expect().Status(http.StatusOK).JSON().IsEqual(own)
}

// A real session id under the wrong lifter is a 404.
//
// This is the leak the endpoint is shaped to prevent: the session exists, the
// lifter exists, and they have nothing to do with each other. GetSession is
// scoped by (id, user_id), so the pair matches no row — the ownership check is
// pointed at a different owner, not relaxed.
func TestLifterSessionRecapRefusesAMismatchedPair(t *testing.T) {
	id, _ := secondLifter(t, "wrong-owner")

	// A session belonging to the primary account, asked for as though it were
	// the second lifter's.
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(e, sessionID, setID, 5, true)

	// Readable under its real owner...
	e.GET(fmt.Sprintf("/lifters/%d/sessions/%d/recap", primaryUserID(e), sessionID)).
		Expect().Status(http.StatusOK)
	// ...and absent under anybody else's.
	e.GET(fmt.Sprintf("/lifters/%d/sessions/%d/recap", id, sessionID)).
		Expect().Status(http.StatusNotFound)

	// An id that names no account answers the same way, so a caller cannot use
	// this endpoint to learn which ids are accounts.
	e.GET(fmt.Sprintf("/lifters/99999999/sessions/%d/recap", sessionID)).
		Expect().Status(http.StatusNotFound)
}

// primaryUserID reads the suite's owning account id off /me.
func primaryUserID(e *httpexpect.Expect) int {
	return int(e.GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().Value("id").Number().Raw())
}

// ---- one lifter's history, read by another ----

// lifterSessions reads another lifter's history as the caller.
func lifterSessions(e *httpexpect.Expect, lifterID int32, query ...any) *httpexpect.Object {
	req := e.GET(fmt.Sprintf("/lifters/%d/sessions", lifterID))
	for i := 0; i+1 < len(query); i += 2 {
		req = req.WithQuery(fmt.Sprint(query[i]), query[i+1])
	}
	return req.Expect().Status(http.StatusOK).JSON().Object()
}

// The endpoint that makes a profile somewhere to go. Before it, the only route
// to another lifter's recap — the screen where one lifter applauds another —
// was the feed, which is everybody's sessions interleaved.
func TestLifterSessionsListTheirHistory(t *testing.T) {
	id, e := ownLifter(t, "their-history")
	_, dayID := makeProgram(t, id, "Their History Program", false)
	logCleanSession(t, e, dayID)

	page := lifterSessions(expect(t), id)
	items := page.Value("items").Array()
	items.Length().IsEqual(1)

	row := items.Value(0).Object()
	row.HasValue("programDayName", "Day One")
	// The lifts ride along, keyed on the LIFTER rather than on the caller —
	// passing the reader's id to ListSessionExerciseWeights would silently
	// return nothing and every session would claim to be empty.
	row.Value("exercises").Array().Length().IsEqual(1)
	row.Value("exercises").Array().Value(0).Object().HasValue("exerciseName", "Squat")
	row.Value("volumeLb").Number().Gt(0)

	// Totals cover the lifter's whole history, which is the same promise
	// GET /sessions makes and the same figures their profile reports.
	page.HasValue("total", 1)
	page.Value("totalVolumeLb").Number().Gt(0)
}

// The two ids in this handler are whose sessions these are and who is reading.
// Confusing them would serve the caller their own history under somebody else's
// name, which is the bug this asserts against.
func TestLifterSessionsAreNotTheCallersOwn(t *testing.T) {
	subjectID, subject := ownLifter(t, "history-subject")
	// Shared, so this test is about WHOSE history rather than about masking —
	// which TestLifterSessionsMaskAPrivateProgramName owns.
	_, subjectDay := makeProgram(t, subjectID, "Subject Program", true)
	logCleanSession(t, subject, subjectDay)

	readerID, reader := ownLifter(t, "history-reader")
	_, readerDay := makeProgram(t, readerID, "Reader Program", true)
	logCleanSession(t, reader, readerDay)

	// The reader asks about the subject and gets the subject's one session —
	// not their own, which is what swapping the two ids would serve.
	items := lifterSessions(reader, subjectID).Value("items").Array()
	items.Length().IsEqual(1)
	items.Value(0).Object().HasValue("programName", "Subject Program")

	// And the reader's own history is still their own.
	mine := lifterSessions(reader, readerID).Value("items").Array()
	mine.Length().IsEqual(1)
	mine.Value(0).Object().HasValue("programName", "Reader Program")
}

// A private program's name is masked for a viewer who was never shown it, the
// same way the feed masks it. This is the second read in the app that hands a
// lifter a fact about a program they cannot open, and it leaks by exactly the
// same route.
func TestLifterSessionsMaskAPrivateProgramName(t *testing.T) {
	id, e := ownLifter(t, "private-program-history")
	_, dayID := makeProgram(t, id, "Nobody Else's Business", false)
	logCleanSession(t, e, dayID)

	// The owner reads their own and sees the real name.
	lifterSessions(e, id).Value("items").Array().
		Value(0).Object().HasValue("programName", "Nobody Else's Business")

	// A stranger gets the category, not the name — and the DAY name is
	// deliberately not masked, because what somebody trained is the point of
	// these screens.
	row := lifterSessions(expect(t), id).Value("items").Array().Value(0).Object()
	row.HasValue("programName", "Custom program")
	row.HasValue("programDayName", "Day One")
}

// Sharing it makes the name legible again, from the same predicate the program
// endpoints use.
func TestLifterSessionsShowASharedProgramName(t *testing.T) {
	id, e := ownLifter(t, "shared-program-history")
	programID, dayID := makeProgram(t, id, "Everybody's Business", true)
	logCleanSession(t, e, dayID)

	lifterSessions(expect(t), id).Value("items").Array().
		Value(0).Object().HasValue("programName", "Everybody's Business")

	// And un-sharing it takes the name back.
	setShared(t, programID, false)
	lifterSessions(expect(t), id).Value("items").Array().
		Value(0).Object().HasValue("programName", "Custom program")
}

// An account that has never logged a rep has an empty history rather than a
// missing one, and it serializes as [] so a surface branches on length.
func TestLifterSessionsAreEmptyForANewAccount(t *testing.T) {
	id, _ := ownLifter(t, "never-trained-history")

	page := lifterSessions(expect(t), id)
	page.Value("items").Array().IsEmpty()
	page.HasValue("total", 0)
}

// An id that names nobody is an answer, not an empty report — the same reason
// lifterFromPath exists at all.
func TestLifterSessionsForAnUnknownLifterAreNotFound(t *testing.T) {
	expect(t).GET("/lifters/99999999/sessions").
		Expect().Status(http.StatusNotFound)
}

func TestLifterSessionsRejectAnImpossibleLimit(t *testing.T) {
	id, _ := ownLifter(t, "history-bad-limit")
	expect(t).GET(fmt.Sprintf("/lifters/%d/sessions", id)).
		WithQuery("limit", 101).
		Expect().Status(http.StatusBadRequest)
}
