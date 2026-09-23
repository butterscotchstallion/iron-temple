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
// The tests below are mostly about the SECOND half, because it is the one with a
// hole to fall into. A missed check on a read path is not a cosmetic bug: the
// prescription of a program is what it prescribes, and POST /sessions would
// materialize a whole workout against one this lifter was never shown.
//
// There is no create endpoint yet — that is the next phase — so custom programs
// are inserted directly. That is deliberate rather than a stopgap: these are
// assertions about the READ path, and building the rows by hand keeps them from
// passing merely because a create handler happened to write what they expected.

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
