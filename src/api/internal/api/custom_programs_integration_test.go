package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// Ownership and visibility of programs, as 0029 defines them.
//
// The whole design rests on one column: programs.created_by_user_id, NULL for
// the install's seeded catalogue and a user id for a program somebody built.
// Everything else falls out of it — the seeded programs are uneditable because
// every write scopes on that column and NULL matches nobody, and a private
// program is invisible because every read scopes on it too.
//
// The visibility half is the one with a hole to fall into, so it has most of the
// tests. A missed check on a read path is not a cosmetic bug: a prescription is
// what a program prescribes, and POST /sessions would materialize a whole workout
// against one this lifter was never shown.
//
// The first half of this file builds its programs by direct INSERT rather than
// through POST /programs, and keeps doing so now that the endpoint exists. That
// is deliberate: those are assertions about the READ path, and going through the
// writer would let them pass merely because the writer happened to produce what
// they expected. The create/clone/archive tests further down do the opposite, and
// exercise the endpoint.

// makeProgram inserts a program owned by a lifter, with one day and one
// prescribed lift, and returns the program and day ids.
//
// Squat at 95 lb because the seeded programs use it: a lift with a history
// behind it is the case where a custom program has to keep prescribing sensibly,
// and reusing the movement rather than inventing one is what exercises that.
func makeProgram(t *testing.T, ownerID int32, name string, shared bool) (programID, dayID int) {
	t.Helper()
	ctx := context.Background()

	err := testPool.QueryRow(ctx,
		`INSERT INTO programs (name, description, created_by_user_id, is_shared)
		 VALUES ($1, 'Built in a test', $2, $3) RETURNING id`,
		name, ownerID, shared).Scan(&programID)
	if err != nil {
		t.Fatalf("insert program %q: %v", name, err)
	}
	// The whole package shares one database, so a program left behind is a
	// program every later test counting /programs will see.
	//
	// Sessions go FIRST and explicitly. sessions.program_day_id has no ON DELETE
	// clause — it RESTRICTs, which is the same fact that makes archiving rather
	// than deleting the right answer for the feature itself — so deleting the
	// program while a test's session points at one of its days fails outright.
	// Swallowing that error would leave both rows behind and turn this helper
	// into a pollution source that only shows up as somebody else's failing
	// assertion about a seed count.
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := testPool.Exec(ctx,
			`DELETE FROM sessions s USING program_days pd
			 WHERE pd.id = s.program_day_id AND pd.program_id = $1`, programID); err != nil {
			t.Errorf("clean up sessions for program %d: %v", programID, err)
		}
		if _, err := testPool.Exec(ctx,
			"DELETE FROM programs WHERE id = $1", programID); err != nil {
			t.Errorf("clean up program %d: %v", programID, err)
		}
	})

	err = testPool.QueryRow(ctx,
		`INSERT INTO program_days (program_id, name, position) VALUES ($1, 'Day One', 1)
		 RETURNING id`, programID).Scan(&dayID)
	if err != nil {
		t.Fatalf("insert program day: %v", err)
	}

	_, err = testPool.Exec(ctx,
		`INSERT INTO program_day_exercises
		     (program_day_id, exercise_id, position, sets, reps, starting_weight_lb)
		 SELECT $1, e.id, 1, 5, 5, 95 FROM exercises e WHERE e.name = 'Squat'`, dayID)
	if err != nil {
		t.Fatalf("insert prescription: %v", err)
	}
	return programID, dayID
}

// setShared flips a program's visibility, which is the only thing un-sharing
// does to the database — the grandfathering it does NOT do is the subject of
// TestUnsharingGrandfathersExistingLifter below.
func setShared(t *testing.T, programID int, shared bool) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		"UPDATE programs SET is_shared = $1 WHERE id = $2", shared, programID); err != nil {
		t.Fatalf("set is_shared on %d: %v", programID, err)
	}
}

func setArchived(t *testing.T, programID int, archived bool) {
	t.Helper()
	value := "NULL"
	if archived {
		value = "now()"
	}
	if _, err := testPool.Exec(context.Background(),
		fmt.Sprintf("UPDATE programs SET archived_at = %s WHERE id = $1", value),
		programID); err != nil {
		t.Fatalf("archive %d: %v", programID, err)
	}
}

// primaryID reads the signed-in primary lifter's id, which the suite otherwise
// never needs — every other test addresses them through their cookie.
func primaryID(t *testing.T) int32 {
	t.Helper()
	return int32(expect(t).GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().Value("id").Number().Raw())
}

// ownLifter is secondLifter for the tests below that LOG SETS.
//
// The package shares one database and one primary lifter, and lift history is
// keyed on (exercise, lifter) across every program — which is the property this
// feature depends on and also the reason these tests must not use the primary
// account. A squat session logged here would land in the same history the
// layoff, recap and exercise-listing tests read, and would surface as a wrong
// number in an assertion three files away rather than as a failure here.
//
// Deleting the account takes its sessions with it (ON DELETE CASCADE from
// users), so the history goes when the test does.
func ownLifter(t *testing.T, username string) (int32, *httpexpect.Expect) {
	t.Helper()
	id, token := secondLifter(t, username)
	return id, expectAs(t, token)
}

// logCleanSession starts a session on a day, completes every set at its target,
// and finishes it — the "everything went to plan" input the linear engine reads
// as a success.
func logCleanSession(t *testing.T, e *httpexpect.Expect, dayID int) {
	t.Helper()
	created := e.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated).JSON().Object()
	sessionID := int(created.Value("id").Number().Raw())

	// The sets are read back from the session rather than assumed. The
	// prescription says 5x5, but what the engine reads is what was actually
	// materialized, and those are different claims.
	sets := created.Value("sets").Array()
	sets.Length().IsEqual(5)
	for _, v := range sets.Iter() {
		setID := int(v.Object().Value("id").Number().Raw())
		e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, setID)).
			WithJSON(map[string]any{"actualReps": 5, "completed": true}).
			Expect().Status(http.StatusOK)
	}
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)
}

// findProgram returns one program's card from the picker, or nil if it is absent.
func findProgram(list *httpexpect.Array, id int) *httpexpect.Object {
	for _, v := range list.Iter() {
		obj := v.Object()
		if int(obj.Value("id").Number().Raw()) == id {
			return obj
		}
	}
	return nil
}

func TestSeededProgramsHaveNoOwner(t *testing.T) {
	// The assertion the whole design rests on. A seeded program with an owner
	// would be one lifter's to rewrite, and the install's catalogue would have
	// quietly become somebody's personal program.
	list := expect(t).GET("/programs").Expect().Status(http.StatusOK).JSON().Array()
	list.Length().Gt(0)

	for _, v := range list.Iter() {
		p := v.Object()
		p.Value("ownerId").IsNull()
		p.HasValue("isMine", false)
		// Seeded programs are the install's, so everyone sees them...
		p.HasValue("isShared", true)
		// ...and none of them ships retired.
		p.Value("archivedAt").IsNull()
	}
}

func TestOwnProgramIsMineAndPrivate(t *testing.T) {
	owner := primaryID(t)
	programID, _ := makeProgram(t, owner, "Private Test Program", false)

	card := findProgram(
		expect(t).GET("/programs").Expect().Status(http.StatusOK).JSON().Array(),
		programID,
	)
	if card == nil {
		t.Fatalf("own program %d absent from the picker", programID)
	}
	card.HasValue("ownerId", owner)
	card.HasValue("isMine", true)
	card.HasValue("isShared", false)
}

// TestPrivateProgramIsInvisible is the important one.
//
// It walks every route that takes a program or a day and asserts a 404 from a
// lifter who was never shown it — 404 and not 403, because confirming that an id
// is valid is already a leak, and that is the rule the rest of the API follows.
//
// POST /sessions is the one that matters most and the one most easily missed. It
// takes the day id in the BODY rather than the path, so it does not go through
// programDay() and inherits none of that helper's checks; and it is a WRITE, so
// an unguarded version would not merely reveal a prescription but materialize a
// whole session against somebody else's private program.
func TestPrivateProgramIsInvisible(t *testing.T) {
	owner := primaryID(t)
	programID, dayID := makeProgram(t, owner, "Invisible Test Program", false)

	_, followerToken := secondLifter(t, "nosy-lifter")
	e := expectAs(t, followerToken)

	if card := findProgram(
		e.GET("/programs").Expect().Status(http.StatusOK).JSON().Array(), programID,
	); card != nil {
		t.Fatalf("private program %d is listed for another lifter", programID)
	}

	e.GET(fmt.Sprintf("/programs/%d", programID)).
		Expect().Status(http.StatusNotFound)
	e.GET(fmt.Sprintf("/programs/%d/next-sessions", programID)).
		Expect().Status(http.StatusNotFound)
	e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusNotFound)
	// The overlay hangs off the day, so it is gated by the same helper.
	e.POST(fmt.Sprintf("/programs/%d/days/%d/assistance", programID, dayID)).
		WithJSON(map[string]any{"exerciseId": 1, "sets": 3, "reps": 10}).
		Expect().Status(http.StatusNotFound)
	// And the write that reaches it without a path to be checked.
	e.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusNotFound)

	// Pinning it as your current program is the quiet version of the same
	// reach: the foreign key alone would accept it, and home would then render a
	// program the API refuses to return.
	e.PATCH("/me").
		WithJSON(map[string]any{"currentProgramId": programID}).
		Expect().Status(http.StatusBadRequest)
}

func TestSharedProgramIsVisibleToOthers(t *testing.T) {
	owner := primaryID(t)
	programID, dayID := makeProgram(t, owner, "Shared Test Program", true)

	_, followerToken := secondLifter(t, "shared-follower")
	e := expectAs(t, followerToken)

	card := findProgram(
		e.GET("/programs").Expect().Status(http.StatusOK).JSON().Array(), programID,
	)
	if card == nil {
		t.Fatalf("shared program %d is not listed for another lifter", programID)
	}
	// Theirs to see, not theirs to edit.
	card.HasValue("isMine", false)
	card.HasValue("ownerId", owner)
	card.HasValue("ownerName", "Primary Lifter")

	e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusOK)
	e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK)
}

// TestUnsharingGrandfathersExistingLifter pins the rule that separates discovery
// from access.
//
// Un-sharing means "stop new people finding it". It cannot sensibly mean "lock
// out the lifter who has been running it for three months": their sessions point
// at its days for ever either way, so taking access away costs them their
// training history's context and buys the owner nothing.
func TestUnsharingGrandfathersExistingLifter(t *testing.T) {
	owner := primaryID(t)
	programID, dayID := makeProgram(t, owner, "Unshared Test Program", true)

	_, trainedToken := secondLifter(t, "grandfathered-lifter")
	trained := expectAs(t, trainedToken)
	trained.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated)

	_, strangerToken := secondLifter(t, "late-lifter")
	stranger := expectAs(t, strangerToken)

	setShared(t, programID, false)

	// The lifter who has trained it keeps it, on every path...
	trained.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusOK)
	trained.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK)
	trained.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated)

	// ...but it is gone from the picker, for them as for everyone. Discovery is
	// what un-sharing governs, and they already know where it is.
	if card := findProgram(
		trained.GET("/programs").Expect().Status(http.StatusOK).JSON().Array(), programID,
	); card != nil {
		t.Fatalf("un-shared program %d is still listed", programID)
	}

	// And the lifter who never trained it loses it outright.
	stranger.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusNotFound)
	stranger.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusNotFound)
}

// TestArchivedProgramLeavesThePickerButStaysTrainable pins the other half of
// discovery-not-access.
//
// users.current_program_id is ON DELETE SET NULL, which archiving does not fire
// — so a lifter part-way through an archived program still points at it. If an
// archived program 404'd, their home screen would become a permanent "couldn't
// load this program" with a retry that can never succeed.
func TestArchivedProgramLeavesThePickerButStaysTrainable(t *testing.T) {
	owner, e := ownLifter(t, "archiving-lifter")
	programID, dayID := makeProgram(t, owner, "Archived Test Program", true)

	setArchived(t, programID, true)

	if card := findProgram(
		e.GET("/programs").Expect().Status(http.StatusOK).JSON().Array(), programID,
	); card != nil {
		t.Fatalf("archived program %d is still on the picker", programID)
	}

	// The owner can still find it when they ask for it by name.
	card := findProgram(
		e.GET("/programs").WithQuery("includeArchived", "true").
			Expect().Status(http.StatusOK).JSON().Array(),
		programID,
	)
	if card == nil {
		t.Fatalf("archived program %d absent even with includeArchived", programID)
	}
	card.Value("archivedAt").NotNull()

	// And it still resolves, still previews, and can still be trained.
	e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusOK)
	e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK)
	e.POST("/sessions").WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated)
}

// TestIncludeArchivedNeverWidensPastTheCaller: asking for archived programs is
// asking for YOUR archived programs. A shared one that has been archived is one
// its owner retired, and retiring it is exactly the decision to stop offering it
// to other people — so the flag must not be a way around that.
func TestIncludeArchivedNeverWidensPastTheCaller(t *testing.T) {
	owner := primaryID(t)
	programID, _ := makeProgram(t, owner, "Retired Shared Program", true)
	setArchived(t, programID, true)

	_, followerToken := secondLifter(t, "archive-peeker")
	list := expectAs(t, followerToken).GET("/programs").
		WithQuery("includeArchived", "true").
		Expect().Status(http.StatusOK).JSON().Array()

	if card := findProgram(list, programID); card != nil {
		t.Fatalf("another lifter's archived program %d surfaced via includeArchived", programID)
	}
}

// TestCustomProgramProgresses is the end-to-end proof that the progression
// engine needed no change at all.
//
// A custom program's prescription is read by the same query, fed to the same
// engine, against a history keyed on (exercise, lifter) that never joins the
// program. So a squat in a program somebody built this morning advances exactly
// as a squat in StrongLifts does — and, more to the point, picks up the weight
// the lifter was already squatting rather than restarting at the seed.
func TestCustomProgramProgresses(t *testing.T) {
	owner, e := ownLifter(t, "progressing-lifter")
	programID, dayID := makeProgram(t, owner, "Progressing Test Program", false)

	logCleanSession(t, e, dayID)

	// A successful session of a barbell lift earns +5.
	status, weight := firstExercisePreview(e, programID, dayID)
	if status != "advance" {
		t.Fatalf("after a clean session the engine said %q, want advance", status)
	}
	if weight != 100 {
		t.Fatalf("next squat is %v lb, want 100", weight)
	}
}

// TestCustomProgramInheritsLiftHistory is the property worth having a test of
// its own for, because it is the reason building your own program is not a
// punishment: history follows the LIFT, not the program.
//
// The lifter here has already squatted 100 in the program above. A brand-new
// program prescribing a squat from 95 must not send them back to 95.
func TestCustomProgramInheritsLiftHistory(t *testing.T) {
	owner, e := ownLifter(t, "inheriting-lifter")

	// Establish a squat history on one program...
	firstID, firstDay := makeProgram(t, owner, "History Source Program", false)
	logCleanSession(t, e, firstDay)
	_, earned := firstExercisePreview(e, firstID, firstDay)

	// ...then build a second program prescribing the same lift from the same
	// seed, and confirm it asks for what was earned rather than for the seed.
	secondID, secondDay := makeProgram(t, owner, "History Heir Program", false)
	_, inherited := firstExercisePreview(e, secondID, secondDay)

	if inherited != earned {
		t.Fatalf("new program prescribes %v lb; the lift was left at %v", inherited, earned)
	}
	if inherited <= 95 {
		t.Fatalf("new program restarted the squat at %v lb rather than carrying it", inherited)
	}
}

// TestSharedProgramSurvivesAFollowersAssistanceCollision pins the guard that
// makes shared programs safe to edit at all.
//
// A follower adds curls to a day as assistance. The owner later promotes curls
// into the program's own prescription — which they are entitled to do, and which
// they cannot even see the follower's overlay to avoid. prescribe() already
// drops an assistance row naming a lift the day prescribes, and without that the
// follower's session would try to insert two sets numbered 1 for the same lift,
// trip session_sets' UNIQUE, and turn every attempt to start the workout into a
// 500 — for ever, with no way for the follower to work out why.
func TestSharedProgramSurvivesAFollowersAssistanceCollision(t *testing.T) {
	owner := primaryID(t)
	programID, dayID := makeProgram(t, owner, "Collision Test Program", true)

	_, followerToken := secondLifter(t, "collision-follower")
	follower := expectAs(t, followerToken)

	curlID := int(expect(t).GET("/exercises").Expect().Status(http.StatusOK).
		JSON().Array().Find(func(_ int, value *httpexpect.Value) bool {
		return value.Object().Value("name").String().Raw() == "Barbell Curl"
	}).Object().Value("id").Number().Raw())

	follower.POST(fmt.Sprintf("/programs/%d/days/%d/assistance", programID, dayID)).
		WithJSON(map[string]any{"exerciseId": curlID, "sets": 3, "reps": 10}).
		Expect().Status(http.StatusCreated)

	// The owner promotes the same lift into the prescription. Done directly
	// because the editor is a later phase; what is under test is what happens
	// NEXT, which is the same either way.
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO program_day_exercises
		     (program_day_id, exercise_id, position, sets, reps, starting_weight_lb)
		 VALUES ($1, $2, 2, 3, 10, 45)`, dayID, curlID); err != nil {
		t.Fatalf("promote curl into the prescription: %v", err)
	}

	// The follower's preview names the curl once, not twice.
	exercises := follower.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("exercises").Array()
	var curls int
	for _, v := range exercises.Iter() {
		if int(v.Object().Value("exerciseId").Number().Raw()) == curlID {
			curls++
		}
	}
	if curls != 1 {
		t.Fatalf("follower's preview lists the curl %d times, want 1", curls)
	}

	// And the session they start from it is creatable rather than a 500.
	follower.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated)
}

// ---------------------------------------------------------------------------
// Creating, cloning and archiving — POST /programs and friends.
// ---------------------------------------------------------------------------

// createProgram posts a new program and returns the created object.
func createProgram(e *httpexpect.Expect, body map[string]any) *httpexpect.Object {
	return e.POST("/programs").WithJSON(body).
		Expect().Status(http.StatusCreated).JSON().Object()
}

// dropProgram registers the cleanup makeProgram does, for programs created
// through the API rather than inserted.
func dropProgram(t *testing.T, programID int) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := testPool.Exec(ctx,
			`DELETE FROM sessions s USING program_days pd
			 WHERE pd.id = s.program_day_id AND pd.program_id = $1`, programID); err != nil {
			t.Errorf("clean up sessions for program %d: %v", programID, err)
		}
		if _, err := testPool.Exec(ctx,
			"DELETE FROM programs WHERE id = $1", programID); err != nil {
			t.Errorf("clean up program %d: %v", programID, err)
		}
	})
}

// seededProgram returns one seeded program's summary by name.
func seededProgram(t *testing.T, e *httpexpect.Expect, name string) *httpexpect.Object {
	t.Helper()
	for _, v := range e.GET("/programs").Expect().Status(http.StatusOK).JSON().Array().Iter() {
		if v.Object().Value("name").String().Raw() == name {
			return v.Object()
		}
	}
	t.Fatalf("seeded program %q not found", name)
	return nil
}

func TestCreateBlankProgram(t *testing.T) {
	_, e := ownLifter(t, "blank-builder")

	created := createProgram(e, map[string]any{
		"name": "Blank Test Program", "description": "  Built from nothing  ",
	})
	dropProgram(t, int(created.Value("id").Number().Raw()))

	created.HasValue("isMine", true)
	created.HasValue("isShared", false) // private by default
	created.Value("archivedAt").IsNull()
	// Trimmed, not stored with the spaces the form sent.
	created.HasValue("description", "Built from nothing")
	// A program with no days is a legitimate state: it is what the editor opens
	// on, and the response has to say so rather than omit the key.
	created.Value("days").Array().IsEmpty()
}

// TestCloneCopiesThePrescriptionAndLeavesTheSourceAlone is the one that would
// catch a clone written as an UPDATE, or one that reparented the source's rows
// instead of copying them — which would silently rewrite StrongLifts for every
// account on the install.
func TestCloneCopiesThePrescriptionAndLeavesTheSourceAlone(t *testing.T) {
	_, e := ownLifter(t, "cloning-lifter")

	sourceID := int(seededProgram(t, e, "StrongLifts 5x5").Value("id").Number().Raw())
	before := e.GET(fmt.Sprintf("/programs/%d", sourceID)).
		Expect().Status(http.StatusOK).JSON().Object().Raw()

	clone := createProgram(e, map[string]any{
		"name": "My StrongLifts", "cloneFromProgramId": sourceID,
	})
	dropProgram(t, int(clone.Value("id").Number().Raw()))

	sourceDays := e.GET(fmt.Sprintf("/programs/%d", sourceID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("days").Array()
	cloneDays := clone.Value("days").Array()
	cloneDays.Length().IsEqual(len(sourceDays.Raw()))

	for i, v := range sourceDays.Iter() {
		src := v.Object()
		dst := cloneDays.Value(i).Object()
		dst.HasValue("name", src.Value("name").String().Raw())
		dst.HasValue("position", src.Value("position").Number().Raw())

		// The weekday is deliberately NOT copied: a schedule is a decision about
		// your own week, and a clone that arrives pre-booked has made it for you.
		dst.Value("weekday").IsNull()

		srcLifts := src.Value("exercises").Array()
		dstLifts := dst.Value("exercises").Array()
		dstLifts.Length().IsEqual(len(srcLifts.Raw()))
		for j, lift := range srcLifts.Iter() {
			from := lift.Object()
			to := dstLifts.Value(j).Object()
			to.HasValue("exerciseId", from.Value("exerciseId").Number().Raw())
			to.HasValue("sets", from.Value("sets").Number().Raw())
			to.HasValue("reps", from.Value("reps").Number().Raw())
			to.HasValue("startingWeightLb", from.Value("startingWeightLb").Number().Raw())
		}
	}

	// And the source is byte-identical to what it was. This is the assertion the
	// whole feature has to keep earning: the install's catalogue is canonical,
	// and a clone is a copy rather than a move.
	e.GET(fmt.Sprintf("/programs/%d", sourceID)).
		Expect().Status(http.StatusOK).JSON().Object().IsEqual(before)
}

// TestCloningMadcowIsRefused: Madcow prescribes percentages of a top set in
// program_day_exercise_sets, and the editor has no way to express or change one.
// A clone would be a program whose ramps its owner can see the effects of and
// never edit — and one bad day-delete away from a lift with no 100% set at all,
// which leaves the engine with no reference day and silently reading every day's
// history as one series.
func TestCloningMadcowIsRefused(t *testing.T) {
	_, e := ownLifter(t, "madcow-cloner")

	madcowID := int(seededProgram(t, e, "Madcow 5x5").Value("id").Number().Raw())
	e.POST("/programs").
		WithJSON(map[string]any{"name": "My Madcow", "cloneFromProgramId": madcowID}).
		Expect().Status(http.StatusConflict).
		JSON().Object().HasValue("code", "unsupported_progression")
}

func TestCloningSomebodyElsesPrivateProgramIs404(t *testing.T) {
	owner := primaryID(t)
	programID, _ := makeProgram(t, owner, "Uncloneable Test Program", false)

	_, e := ownLifter(t, "clone-thief")
	// 404 and not 403: cloning must not become the one path that confirms an id
	// is a real program.
	e.POST("/programs").
		WithJSON(map[string]any{"name": "Stolen", "cloneFromProgramId": programID}).
		Expect().Status(http.StatusNotFound)
}

// TestProgramNamesAreUniquePerOwner pins the property the two partial indexes in
// 0029 exist for, in both directions.
func TestProgramNamesAreUniquePerOwner(t *testing.T) {
	_, first := ownLifter(t, "name-owner")
	_, second := ownLifter(t, "name-neighbour")

	created := createProgram(first, map[string]any{"name": "Push Pull Legs"})
	dropProgram(t, int(created.Value("id").Number().Raw()))

	// The same lifter cannot have two...
	first.POST("/programs").WithJSON(map[string]any{"name": "push pull legs"}).
		Expect().Status(http.StatusConflict).
		JSON().Object().HasValue("code", "duplicate_name")

	// ...nor may anybody shadow a seeded name, which belongs to the install.
	first.POST("/programs").WithJSON(map[string]any{"name": "StrongLifts 5x5"}).
		Expect().Status(http.StatusConflict).
		JSON().Object().HasValue("code", "duplicate_name")

	// ...but a DIFFERENT lifter may use the same name, which is the whole point
	// of per-owner uniqueness: the picker tells them apart by owner.
	mine := createProgram(second, map[string]any{"name": "Push Pull Legs"})
	dropProgram(t, int(mine.Value("id").Number().Raw()))
}

func TestUpdateProgramRenamesAndShares(t *testing.T) {
	_, e := ownLifter(t, "renaming-lifter")
	created := createProgram(e, map[string]any{"name": "Before Rename"})
	programID := int(created.Value("id").Number().Raw())
	dropProgram(t, programID)

	updated := e.PATCH(fmt.Sprintf("/programs/%d", programID)).
		WithJSON(map[string]any{"name": "After Rename", "isShared": true}).
		Expect().Status(http.StatusOK).JSON().Object()
	updated.HasValue("name", "After Rename")
	updated.HasValue("isShared", true)

	// Omitted fields are left alone, and re-saving the same name is not a
	// conflict with itself.
	e.PATCH(fmt.Sprintf("/programs/%d", programID)).
		WithJSON(map[string]any{"name": "After Rename"}).
		Expect().Status(http.StatusOK).
		JSON().Object().HasValue("isShared", true)
}

// TestSeededProgramsRefuseEveryWrite is the assertion that the narrowing held.
//
// Nothing about these handlers says "if seeded, refuse". They scope on
// created_by_user_id = caller, and a seeded program has NULL there — so this
// passes because of how the queries are written rather than because of a check
// somebody remembered.
func TestSeededProgramsRefuseEveryWrite(t *testing.T) {
	_, e := ownLifter(t, "seed-vandal")
	seededID := int(seededProgram(t, e, "StrongLifts 5x5").Value("id").Number().Raw())

	e.PATCH(fmt.Sprintf("/programs/%d", seededID)).
		WithJSON(map[string]any{"name": "Mine Now"}).
		Expect().Status(http.StatusNotFound)
	e.POST(fmt.Sprintf("/programs/%d/archive", seededID)).
		Expect().Status(http.StatusNotFound)
	e.DELETE(fmt.Sprintf("/programs/%d/archive", seededID)).
		Expect().Status(http.StatusNotFound)
}

func TestAnotherLiftersSharedProgramRefusesEveryWrite(t *testing.T) {
	owner := primaryID(t)
	programID, _ := makeProgram(t, owner, "Read Only Test Program", true)

	_, e := ownLifter(t, "shared-meddler")
	// Readable...
	e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusOK)
	// ...and not writable, as a 404 rather than a 403.
	e.PATCH(fmt.Sprintf("/programs/%d", programID)).
		WithJSON(map[string]any{"name": "Meddled"}).
		Expect().Status(http.StatusNotFound)
	e.POST(fmt.Sprintf("/programs/%d/archive", programID)).
		Expect().Status(http.StatusNotFound)
}

// TestArchiveRoundTripKeepsASession is the RESTRICT case, end to end: the reason
// this archives rather than deletes.
func TestArchiveRoundTripKeepsASession(t *testing.T) {
	owner, e := ownLifter(t, "archive-round-tripper")
	programID, dayID := makeProgram(t, owner, "Round Trip Test Program", false)

	session := e.POST("/sessions").WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated).JSON().Object()
	sessionID := int(session.Value("id").Number().Raw())

	e.POST(fmt.Sprintf("/programs/%d/archive", programID)).
		Expect().Status(http.StatusNoContent)

	// The session is untouched — which is the whole reason a program cannot be
	// deleted, and therefore the reason this endpoint exists at all.
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusOK)

	e.DELETE(fmt.Sprintf("/programs/%d/archive", programID)).
		Expect().Status(http.StatusNoContent)
	card := findProgram(
		e.GET("/programs").Expect().Status(http.StatusOK).JSON().Array(), programID,
	)
	if card == nil {
		t.Fatalf("program %d did not come back from the archive", programID)
	}
	card.Value("archivedAt").IsNull()
}

// TestFeedMasksAPrivateProgramName covers the leak that is invisible from inside
// this feature: nothing in the program code touches the feed, so a private
// program's name would reach every lifter on the install the moment its owner
// logged a session.
func TestFeedMasksAPrivateProgramName(t *testing.T) {
	owner, ownerClient := ownLifter(t, "feed-private-owner")
	programID, dayID := makeProgram(t, owner, "Secret Squat Cycle", false)
	logCleanSession(t, ownerClient, dayID)

	_, viewer := ownLifter(t, "feed-onlooker")
	entry := findFeedEntry(t, viewer, dayID)
	entry.HasValue("programName", "Custom program")
	// The DAY name is deliberately not masked: a feed card is about what somebody
	// trained, which is the point of a feed, and "Custom program · " with nothing
	// after it is a card not worth drawing.
	entry.HasValue("programDayName", "Day One")

	// Sharing it makes the name public, because that is what sharing means.
	ownerClient.PATCH(fmt.Sprintf("/programs/%d", programID)).
		WithJSON(map[string]any{"isShared": true}).
		Expect().Status(http.StatusOK)
	findFeedEntry(t, viewer, dayID).HasValue("programName", "Secret Squat Cycle")
}

// findFeedEntry returns the feed row for a session on the given day.
func findFeedEntry(t *testing.T, e *httpexpect.Expect, dayID int) *httpexpect.Object {
	t.Helper()
	items := e.GET("/feed").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array()
	for _, v := range items.Iter() {
		if int(v.Object().Value("programDayId").Number().Raw()) == dayID {
			return v.Object()
		}
	}
	t.Fatalf("no feed entry for day %d", dayID)
	return nil
}

// TestLifterRecapMasksAPrivateProgramName is the second leak, and the subtler
// one: buildSessionRecap is scoped to the lifter whose session it is, so unlike
// every self-scoped caller of GetSession it can hand the VIEWER a program name
// they were never shown.
func TestLifterRecapMasksAPrivateProgramName(t *testing.T) {
	owner, ownerClient := ownLifter(t, "recap-private-owner")
	_, dayID := makeProgram(t, owner, "Secret Bench Cycle", false)
	logCleanSession(t, ownerClient, dayID)

	sessionID := int(ownerClient.GET("/sessions").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array().Value(0).Object().
		Value("id").Number().Raw())

	_, viewer := ownLifter(t, "recap-onlooker")
	viewer.GET(fmt.Sprintf("/lifters/%d/sessions/%d/recap", owner, sessionID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("session").Object().
		HasValue("programName", "Custom program")

	// The owner reading their own recap sees the real name.
	ownerClient.GET(fmt.Sprintf("/sessions/%d/recap", sessionID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("session").Object().
		HasValue("programName", "Secret Bench Cycle")
}

// ---------------------------------------------------------------------------
// Editing: days, prescriptions and order.
// ---------------------------------------------------------------------------

// buildEmptyProgram creates a program through the API and returns its id.
func buildEmptyProgram(t *testing.T, e *httpexpect.Expect, name string) int {
	t.Helper()
	created := createProgram(e, map[string]any{"name": name})
	id := int(created.Value("id").Number().Raw())
	dropProgram(t, id)
	return id
}

// addDay posts a day and returns the id of the one that was added.
func addDay(t *testing.T, e *httpexpect.Expect, programID int, name string) int {
	t.Helper()
	days := e.POST(fmt.Sprintf("/programs/%d/days", programID)).
		WithJSON(map[string]any{"name": name}).
		Expect().Status(http.StatusCreated).
		JSON().Object().Value("days").Array()
	for _, v := range days.Iter() {
		if v.Object().Value("name").String().Raw() == name {
			return int(v.Object().Value("id").Number().Raw())
		}
	}
	t.Fatalf("day %q missing from the response", name)
	return 0
}

// exerciseNamed looks up a movement's id from the library.
func exerciseNamed(t *testing.T, e *httpexpect.Expect, name string) int {
	t.Helper()
	for _, v := range e.GET("/exercises").Expect().Status(http.StatusOK).JSON().Array().Iter() {
		if v.Object().Value("name").String().Raw() == name {
			return int(v.Object().Value("id").Number().Raw())
		}
	}
	t.Fatalf("exercise %q not in the library", name)
	return 0
}

// addLift posts a prescription and returns the day it landed on.
func addLift(
	t *testing.T, e *httpexpect.Expect, programID, dayID, exerciseID int, sets, reps int,
) *httpexpect.Object {
	t.Helper()
	program := e.POST(fmt.Sprintf("/programs/%d/days/%d/exercises", programID, dayID)).
		WithJSON(map[string]any{
			"exerciseId": exerciseID, "sets": sets, "reps": reps, "startingWeightLb": 95,
		}).
		Expect().Status(http.StatusCreated).JSON().Object()
	return dayIn(t, program, dayID)
}

// dayIn plucks one day out of a Program response.
func dayIn(t *testing.T, program *httpexpect.Object, dayID int) *httpexpect.Object {
	t.Helper()
	for _, v := range program.Value("days").Array().Iter() {
		if int(v.Object().Value("id").Number().Raw()) == dayID {
			return v.Object()
		}
	}
	t.Fatalf("day %d missing from the program", dayID)
	return nil
}

// liftIDs reads a day's prescribed exercise row ids, in order.
func liftIDs(day *httpexpect.Object) []int {
	var out []int
	for _, v := range day.Value("exercises").Array().Iter() {
		out = append(out, int(v.Object().Value("id").Number().Raw()))
	}
	return out
}

func TestBuildAProgramFromNothing(t *testing.T) {
	_, e := ownLifter(t, "day-builder")
	programID := buildEmptyProgram(t, e, "Hand Built Program")

	dayID := addDay(t, e, programID, "Push")
	squat := exerciseNamed(t, e, "Squat")
	day := addLift(t, e, programID, dayID, squat, 5, 5)

	lifts := day.Value("exercises").Array()
	lifts.Length().IsEqual(1)
	lifts.Value(0).Object().HasValue("exerciseName", "Squat")
	lifts.Value(0).Object().HasValue("sets", 5)

	// And it is immediately trainable, which is the point of the whole exercise.
	e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("exercises").Array().Length().IsEqual(1)
}

func TestTheSameLiftCannotBeAddedTwice(t *testing.T) {
	_, e := ownLifter(t, "double-adder")
	programID := buildEmptyProgram(t, e, "Duplicate Lift Program")
	dayID := addDay(t, e, programID, "Day One")
	squat := exerciseNamed(t, e, "Squat")
	addLift(t, e, programID, dayID, squat, 5, 5)

	// "3x8 and also 3x10" is one entry with the sets edited, which is the rule
	// program_day_assistance states and this table has had since 0001.
	e.POST(fmt.Sprintf("/programs/%d/days/%d/exercises", programID, dayID)).
		WithJSON(map[string]any{"exerciseId": squat, "sets": 3, "reps": 8}).
		Expect().Status(http.StatusConflict).
		JSON().Object().HasValue("code", "duplicate_exercise")
}

func TestEditAndRemoveALift(t *testing.T) {
	_, e := ownLifter(t, "lift-editor")
	programID := buildEmptyProgram(t, e, "Editable Program")
	dayID := addDay(t, e, programID, "Day One")
	squat := exerciseNamed(t, e, "Squat")
	day := addLift(t, e, programID, dayID, squat, 5, 5)
	liftID := liftIDs(day)[0]

	updated := e.PATCH(fmt.Sprintf("/programs/%d/days/%d/exercises/%d", programID, dayID, liftID)).
		WithJSON(map[string]any{"sets": 3, "startingWeightLb": 135}).
		Expect().Status(http.StatusOK).JSON().Object()
	lift := dayIn(t, updated, dayID).Value("exercises").Array().Value(0).Object()
	lift.HasValue("sets", 3)
	lift.HasValue("startingWeightLb", 135)
	// Omitted fields are left alone.
	lift.HasValue("reps", 5)

	e.DELETE(fmt.Sprintf("/programs/%d/days/%d/exercises/%d", programID, dayID, liftID)).
		Expect().Status(http.StatusNoContent)
	e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusOK).
		JSON().Object().Value("days").Array().Value(0).Object().
		Value("exercises").Array().IsEmpty()
}

// TestRemovingALiftKeepsItsHistory is why program_day_exercises needs no
// archived_at while program_days does: session_sets references the MOVEMENT, so
// taking a lift off a day loses a plan and never a performance.
func TestRemovingALiftKeepsItsHistory(t *testing.T) {
	owner, e := ownLifter(t, "lift-remover")
	programID, dayID := makeProgram(t, owner, "History Keeping Program", false)
	logCleanSession(t, e, dayID)

	squat := exerciseNamed(t, e, "Squat")
	before := e.GET(fmt.Sprintf("/exercises/%d/history", squat)).
		Expect().Status(http.StatusOK).JSON().Object().Value("points").Array()
	before.Length().Gt(0)

	day := dayIn(t, e.GET(fmt.Sprintf("/programs/%d", programID)).
		Expect().Status(http.StatusOK).JSON().Object(), dayID)
	e.DELETE(fmt.Sprintf("/programs/%d/days/%d/exercises/%d",
		programID, dayID, liftIDs(day)[0])).
		Expect().Status(http.StatusNoContent)

	e.GET(fmt.Sprintf("/exercises/%d/history", squat)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("points").Array().
		Length().IsEqual(len(before.Raw()))
}

// TestReorderRotatesLifts is the case a naive multi-row UPDATE cannot do.
//
// 1,2,3 -> 2,3,1 collides part-way through a single statement against a
// non-deferrable UNIQUE, and WHICH shuffles collide depends on the physical row
// order — so the bug ships green under any shuffle nobody thought to test.
func TestReorderRotatesLifts(t *testing.T) {
	_, e := ownLifter(t, "reordering-lifter")
	programID := buildEmptyProgram(t, e, "Reorderable Program")
	dayID := addDay(t, e, programID, "Day One")

	var ids []int
	for _, name := range []string{"Squat", "Bench Press", "Barbell Row"} {
		day := addLift(t, e, programID, dayID, exerciseNamed(t, e, name), 5, 5)
		ids = liftIDs(day)
	}
	if len(ids) != 3 {
		t.Fatalf("expected three lifts, got %d", len(ids))
	}

	rotated := []int{ids[1], ids[2], ids[0]}
	program := e.PUT(fmt.Sprintf("/programs/%d/days/%d/exercises/order", programID, dayID)).
		WithJSON(map[string]any{"ids": rotated}).
		Expect().Status(http.StatusOK).JSON().Object()

	got := liftIDs(dayIn(t, program, dayID))
	for i, want := range rotated {
		if got[i] != want {
			t.Fatalf("after rotation position %d holds %d, want %d", i+1, got[i], want)
		}
	}
	// And it survives a re-read, so the order is stored rather than merely echoed.
	reread := liftIDs(dayIn(t, e.GET(fmt.Sprintf("/programs/%d", programID)).
		Expect().Status(http.StatusOK).JSON().Object(), dayID))
	for i, want := range rotated {
		if reread[i] != want {
			t.Fatalf("re-read position %d holds %d, want %d", i+1, reread[i], want)
		}
	}
}

// TestPartialReorderIsRefusedAndChangesNothing is the guard against the silent
// corruption the park-and-place approach would otherwise allow: a request naming
// only some of a day's lifts would leave the rest parked at max+n for ever.
func TestPartialReorderIsRefusedAndChangesNothing(t *testing.T) {
	_, e := ownLifter(t, "partial-reorderer")
	programID := buildEmptyProgram(t, e, "Partial Reorder Program")
	dayID := addDay(t, e, programID, "Day One")

	var ids []int
	for _, name := range []string{"Squat", "Bench Press"} {
		day := addLift(t, e, programID, dayID, exerciseNamed(t, e, name), 5, 5)
		ids = liftIDs(day)
	}

	e.PUT(fmt.Sprintf("/programs/%d/days/%d/exercises/order", programID, dayID)).
		WithJSON(map[string]any{"ids": []int{ids[1]}}).
		Expect().Status(http.StatusBadRequest)
	// A repeat is refused too: [7,7] against a two-row day moves one row and
	// reports one, which a bare count comparison would wave through.
	e.PUT(fmt.Sprintf("/programs/%d/days/%d/exercises/order", programID, dayID)).
		WithJSON(map[string]any{"ids": []int{ids[0], ids[0]}}).
		Expect().Status(http.StatusBadRequest)

	// The transaction rolled back, so the original order stands — rather than
	// leaving the day parked in the numbering nothing can read.
	got := liftIDs(dayIn(t, e.GET(fmt.Sprintf("/programs/%d", programID)).
		Expect().Status(http.StatusOK).JSON().Object(), dayID))
	for i, want := range ids {
		if got[i] != want {
			t.Fatalf("after a refused reorder position %d holds %d, want %d", i+1, got[i], want)
		}
	}
}

func TestReorderDays(t *testing.T) {
	_, e := ownLifter(t, "day-reorderer")
	programID := buildEmptyProgram(t, e, "Day Order Program")
	a := addDay(t, e, programID, "Push")
	b := addDay(t, e, programID, "Pull")
	c := addDay(t, e, programID, "Legs")

	// The same rotation, one level up — and a check that /days/order reaches the
	// reorder handler rather than being swallowed by /days/{dayId}.
	program := e.PUT(fmt.Sprintf("/programs/%d/days/order", programID)).
		WithJSON(map[string]any{"ids": []int{b, c, a}}).
		Expect().Status(http.StatusOK).JSON().Object()

	days := program.Value("days").Array()
	days.Value(0).Object().HasValue("id", b)
	days.Value(1).Object().HasValue("id", c)
	days.Value(2).Object().HasValue("id", a)
}

// TestRenamingADayDoesNotClearItsWeekday is the latent bug the second field
// detonates: absent and null are indistinguishable in a bare *int32 decode, so
// a rename would silently unschedule the day.
func TestRenamingADayDoesNotClearItsWeekday(t *testing.T) {
	_, e := ownLifter(t, "day-renamer")
	programID := buildEmptyProgram(t, e, "Renaming Program")
	dayID := addDay(t, e, programID, "Day One")

	e.PATCH(fmt.Sprintf("/programs/%d/days/%d", programID, dayID)).
		WithJSON(map[string]any{"weekday": 1}).
		Expect().Status(http.StatusNoContent)

	e.PATCH(fmt.Sprintf("/programs/%d/days/%d", programID, dayID)).
		WithJSON(map[string]any{"name": "Squat Day"}).
		Expect().Status(http.StatusNoContent)

	day := dayIn(t, e.GET(fmt.Sprintf("/programs/%d", programID)).
		Expect().Status(http.StatusOK).JSON().Object(), dayID)
	day.HasValue("name", "Squat Day")
	day.HasValue("weekday", 1)

	// An explicit null still means "unschedule it", which is the distinction the
	// raw decode exists to keep.
	e.PATCH(fmt.Sprintf("/programs/%d/days/%d", programID, dayID)).
		WithJSON(map[string]any{"weekday": nil}).
		Expect().Status(http.StatusNoContent)
	dayIn(t, e.GET(fmt.Sprintf("/programs/%d", programID)).
		Expect().Status(http.StatusOK).JSON().Object(), dayID).
		Value("weekday").IsNull()
}

// TestSeededProgramDaysStayScheduleableButNotRenameable pins the split rule in
// updateProgramDay, which looks like a bug unless it is asserted on purpose.
func TestSeededProgramDaysStayScheduleableButNotRenameable(t *testing.T) {
	_, e := ownLifter(t, "seed-scheduler")
	seeded := seededProgram(t, e, "StrongLifts 5x5")
	programID := int(seeded.Value("id").Number().Raw())
	dayID := int(e.GET(fmt.Sprintf("/programs/%d", programID)).
		Expect().Status(http.StatusOK).JSON().Object().
		Value("days").Array().Value(0).Object().Value("id").Number().Raw())

	// The weekday picker on the program screen has worked on seeded programs
	// since 0003 and must keep working: they have no owner, so owner-scoping this
	// outright would remove the ability to schedule StrongLifts at all.
	e.PATCH(fmt.Sprintf("/programs/%d/days/%d", programID, dayID)).
		WithJSON(map[string]any{"weekday": 3}).
		Expect().Status(http.StatusNoContent)

	// Renaming is a change to the program, and the program is the install's.
	e.PATCH(fmt.Sprintf("/programs/%d/days/%d", programID, dayID)).
		WithJSON(map[string]any{"name": "Leg Day"}).
		Expect().Status(http.StatusNotFound)

	// As is everything structural.
	e.POST(fmt.Sprintf("/programs/%d/days", programID)).
		WithJSON(map[string]any{"name": "Workout Z"}).
		Expect().Status(http.StatusNotFound)
	e.DELETE(fmt.Sprintf("/programs/%d/days/%d", programID, dayID)).
		Expect().Status(http.StatusNotFound)
	e.POST(fmt.Sprintf("/programs/%d/days/%d/exercises", programID, dayID)).
		WithJSON(map[string]any{"exerciseId": exerciseNamed(t, e, "Squat"), "sets": 5, "reps": 5}).
		Expect().Status(http.StatusNotFound)

	// Put it back, since program_days.weekday is a shared column and the suite
	// runs against one database.
	e.PATCH(fmt.Sprintf("/programs/%d/days/%d", programID, dayID)).
		WithJSON(map[string]any{"weekday": nil}).
		Expect().Status(http.StatusNoContent)
}

// TestRemovingAnUntrainedDayDeletesIt, and its sibling below, are the two halves
// of what the schema allows.
func TestRemovingAnUntrainedDayDeletesIt(t *testing.T) {
	_, e := ownLifter(t, "day-deleter")
	programID := buildEmptyProgram(t, e, "Deletable Day Program")
	dayID := addDay(t, e, programID, "Day One")

	e.DELETE(fmt.Sprintf("/programs/%d/days/%d", programID, dayID)).
		Expect().Status(http.StatusNoContent)
	e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusOK).
		JSON().Object().Value("days").Array().IsEmpty()

	// And the name is free again, which is what the partial unique index buys:
	// an archived or deleted day must not hold its name hostage.
	addDay(t, e, programID, "Day One")
}

// TestRemovingATrainedDayArchivesIt: sessions.program_day_id restricts, so the
// row cannot go. Archiving is the honest version of that, and the session it
// could not delete is the proof.
func TestRemovingATrainedDayArchivesIt(t *testing.T) {
	owner, e := ownLifter(t, "day-archiver")
	programID, dayID := makeProgram(t, owner, "Trained Day Program", false)

	session := e.POST("/sessions").WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated).JSON().Object()
	sessionID := int(session.Value("id").Number().Raw())

	e.DELETE(fmt.Sprintf("/programs/%d/days/%d", programID, dayID)).
		Expect().Status(http.StatusNoContent)

	// Gone from the program...
	e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusOK).
		JSON().Object().Value("days").Array().IsEmpty()
	// ...and the workout performed on it is exactly where it was, still naming
	// the day it was done on.
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusOK).
		JSON().Object().HasValue("programDayName", "Day One")

	// The name is reusable, because the unique index is restricted to live days.
	addDay(t, e, programID, "Day One")
}

func TestPrescriptionBoundsAreEnforced(t *testing.T) {
	_, e := ownLifter(t, "bounds-tester")
	programID := buildEmptyProgram(t, e, "Bounded Program")
	dayID := addDay(t, e, programID, "Day One")
	squat := exerciseNamed(t, e, "Squat")

	for _, body := range []map[string]any{
		{"exerciseId": squat, "sets": 0, "reps": 5},
		{"exerciseId": squat, "sets": 21, "reps": 5},
		{"exerciseId": squat, "sets": 5, "reps": 0},
		{"exerciseId": squat, "sets": 5, "reps": 101},
		// Above this, NUMERIC(6,2) would take it and the next one would 500.
		{"exerciseId": squat, "sets": 5, "reps": 5, "startingWeightLb": 2001},
		{"exerciseId": squat, "sets": 5, "reps": 5, "startingWeightLb": -1},
	} {
		e.POST(fmt.Sprintf("/programs/%d/days/%d/exercises", programID, dayID)).
			WithJSON(body).Expect().Status(http.StatusBadRequest)
	}

	// An exercise nobody can use is rejected rather than 500ing on the key.
	e.POST(fmt.Sprintf("/programs/%d/days/%d/exercises", programID, dayID)).
		WithJSON(map[string]any{"exerciseId": 999999, "sets": 5, "reps": 5}).
		Expect().Status(http.StatusBadRequest)
}
