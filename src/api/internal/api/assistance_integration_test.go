package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// The exercise library and assistance work.
//
// The premise the whole feature rests on is that programs are never edited:
// assistance is a per-user overlay on a shared program day, so a lifter can
// change their plan freely without moving anything the progression engine or the
// Racked recap reads. TestAssistanceLeavesMainLiftProgressionAlone is the test
// that actually pins that claim; the rest cover the mechanics around it.

// addAssistance attaches an exercise to a day and removes it when the test ends,
// so a plan built by one test does not materialize into another's sessions.
func addAssistance(t *testing.T, e *httpexpect.Expect, programID, dayID, exerciseID, sets, reps int, weightLb float64) *httpexpect.Object {
	t.Helper()
	created := e.POST(fmt.Sprintf("/programs/%d/days/%d/assistance", programID, dayID)).
		WithJSON(map[string]any{
			"exerciseId": exerciseID, "sets": sets, "reps": reps, "weightLb": weightLb,
		}).
		Expect().Status(http.StatusCreated).
		JSON().Object()
	id := int(created.Value("id").Number().Raw())
	t.Cleanup(func() {
		// Status deliberately unasserted: several tests below remove the entry
		// themselves, and a 404 here means the cleanup had nothing left to do.
		e.DELETE(fmt.Sprintf("/programs/%d/days/%d/assistance/%d", programID, dayID, id)).Expect()
	})
	return created
}

// exerciseIDByName finds a seeded exercise by name. Names are stable — they are
// what the seed migrations key on — and an id is not.
func exerciseIDByName(t *testing.T, e *httpexpect.Expect, name string) int {
	t.Helper()
	list := e.GET("/exercises").Expect().Status(http.StatusOK).JSON().Array()
	for i := 0; i < int(list.Length().Raw()); i++ {
		ex := list.Value(i).Object()
		if ex.Value("name").String().Raw() == name {
			return int(ex.Value("id").Number().Raw())
		}
	}
	t.Fatalf("no seeded exercise named %q", name)
	return 0
}

// sessionSetsFor returns a session's sets for one exercise, in the order the API
// returned them.
func sessionSetsFor(obj *httpexpect.Object, exerciseID int) []*httpexpect.Object {
	sets := obj.Value("sets").Array()
	var out []*httpexpect.Object
	for i := 0; i < int(sets.Length().Raw()); i++ {
		set := sets.Value(i).Object()
		if int(set.Value("exerciseId").Number().Raw()) == exerciseID {
			out = append(out, set)
		}
	}
	return out
}

func TestLibraryListsAccessoriesWithMetadata(t *testing.T) {
	e := expect(t)

	list := e.GET("/exercises").Expect().Status(http.StatusOK).JSON().Array()

	var sawMain, sawAccessory bool
	for i := 0; i < int(list.Length().Raw()); i++ {
		ex := list.Value(i).Object()
		ex.ContainsKey("muscleGroup").ContainsKey("equipment")
		// Nothing seeded belongs to anyone.
		ex.Value("isCustom").Boolean().IsFalse()
		if ex.Value("name").String().Raw() == "Squat" {
			sawMain = true
			ex.Value("isAccessory").Boolean().IsFalse()
			ex.Value("muscleGroup").String().IsEqual("legs")
			ex.Value("equipment").String().IsEqual("barbell")
		}
		if ex.Value("name").String().Raw() == "Dip" {
			sawAccessory = true
			ex.Value("isAccessory").Boolean().IsTrue()
			ex.Value("equipment").String().IsEqual("bodyweight")
		}
	}
	if !sawMain || !sawAccessory {
		t.Fatalf("library is missing seeded rows: squat=%v dip=%v", sawMain, sawAccessory)
	}
}

// scope=performed is what keeps the Progress page from rendering the whole
// catalogue. It must return only lifts with logged work, and reject anything
// that is not one of the two scopes rather than silently widening.
func TestExerciseScopePerformed(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	set := session.Value("sets").Array().Value(0).Object()
	logged := int(set.Value("exerciseId").Number().Raw())
	logSet(e, sessionID, int(set.Value("id").Number().Raw()), 5, true)

	performed := e.GET("/exercises").WithQuery("scope", "performed").
		Expect().Status(http.StatusOK).JSON().Array()

	all := int(e.GET("/exercises").Expect().Status(http.StatusOK).
		JSON().Array().Length().Raw())
	if n := int(performed.Length().Raw()); n >= all {
		t.Fatalf("scope=performed returned %d of %d exercises; expected a narrower list", n, all)
	}
	for i := 0; i < int(performed.Length().Raw()); i++ {
		if int(performed.Value(i).Object().Value("id").Number().Raw()) == logged {
			return
		}
	}
	t.Fatalf("exercise %d has logged sets but is missing from scope=performed", logged)
}

func TestExerciseScopeRejectsUnknownValue(t *testing.T) {
	expect(t).GET("/exercises").WithQuery("scope", "everything").
		Expect().Status(http.StatusBadRequest)
}

// Assistance is prescribed after the main lifts and materializes into the
// session that follows — the whole point of the feature.
func TestAssistanceIsPrescribedAndMaterialized(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	dipID := exerciseIDByName(t, e, "Dip")

	addAssistance(t, e, programID, dayID, dipID, 3, 8, 0)

	// The preview marks it as assistance, and does not run the engine on it.
	preview := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("exercises").Array()
	last := preview.Value(int(preview.Length().Raw()) - 1).Object()
	last.Value("exerciseId").Number().IsEqual(dipID)
	last.Value("kind").String().IsEqual("assistance")
	last.Value("progression").Object().Value("status").String().IsEqual("fixed")
	// Everything before it is the program's own prescription.
	preview.Value(0).Object().Value("kind").String().IsEqual("main")

	// Starting the session writes the sets.
	session := startSession(t, e, dayID)
	dips := sessionSetsFor(session, dipID)
	if len(dips) != 3 {
		t.Fatalf("materialized %d dip sets, want 3", len(dips))
	}
	for _, set := range dips {
		set.Value("kind").String().IsEqual("assistance")
		set.Value("targetReps").Number().IsEqual(8)
	}

	// And they land after the main lifts, which is where you do them.
	sets := session.Value("sets").Array()
	lastSet := sets.Value(int(sets.Length().Raw()) - 1).Object()
	lastSet.Value("exerciseId").Number().IsEqual(dipID)
}

// The carry-forward rule: assistance is prescribed at the weight last logged for
// the lift, so adding a plate mid-workout is how you progress it.
func TestAssistanceWeightCarriesForward(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	curlID := exerciseIDByName(t, e, "Barbell Curl")

	addAssistance(t, e, programID, dayID, curlID, 2, 10, 0)

	// First time out there is no history, so the configured weight is used.
	first := startSession(t, e, dayID)
	firstID := int(first.Value("id").Number().Raw())
	curls := sessionSetsFor(first, curlID)
	curls[0].Value("weightLb").Number().IsEqual(0)

	// Log it heavier than prescribed, which is the lifter changing their mind.
	for _, set := range curls {
		logSetAt(e, firstID, int(set.Value("id").Number().Raw()), 10, 65, true)
	}

	// The next prescription follows.
	second := startSession(t, e, dayID)
	sessionSetsFor(second, curlID)[0].Value("weightLb").Number().IsEqual(65)

	preview := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("exercises").Array()
	last := preview.Value(int(preview.Length().Raw()) - 1).Object()
	last.Value("weightLb").Number().IsEqual(65)
	// Carried forward, not advanced: the engine did not add five pounds.
	last.Value("progression").Object().Value("status").String().IsEqual("fixed")
}

// The claim the feature is built on. Adding and removing assistance must not
// move the main lifts by a pound, because the progression engine reads a
// prescription that assistance never touches.
func TestAssistanceLeavesMainLiftProgressionAlone(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)

	beforeStatus, beforeWeight := firstExercisePreview(e, programID, dayID)

	rowID := exerciseIDByName(t, e, "Dumbbell Row")
	created := addAssistance(t, e, programID, dayID, rowID, 3, 12, 40)

	withStatus, withWeight := firstExercisePreview(e, programID, dayID)
	if withStatus != beforeStatus || withWeight != beforeWeight {
		t.Fatalf("adding assistance moved the main lift: %s@%v -> %s@%v",
			beforeStatus, beforeWeight, withStatus, withWeight)
	}

	e.DELETE(fmt.Sprintf("/programs/%d/days/%d/assistance/%d",
		programID, dayID, int(created.Value("id").Number().Raw()))).
		Expect().Status(http.StatusNoContent)

	afterStatus, afterWeight := firstExercisePreview(e, programID, dayID)
	if afterStatus != beforeStatus || afterWeight != beforeWeight {
		t.Fatalf("removing assistance moved the main lift: %s@%v -> %s@%v",
			beforeStatus, beforeWeight, afterStatus, afterWeight)
	}
}

// Removing assistance removes the plan, not the performance. This is what the
// COALESCE fallback in ListSessionSets exists for: with the old INNER JOIN the
// logged sets would vanish out of a finished session.
func TestRemovingAssistanceKeepsLoggedSets(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	plankID := exerciseIDByName(t, e, "Plank")

	created := addAssistance(t, e, programID, dayID, plankID, 2, 30, 0)

	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	// A weighted plank, so the work shows up as tonnage rather than as zero,
	// which would make the volume comparison below true for the wrong reason.
	for _, set := range sessionSetsFor(session, plankID) {
		logSetAt(e, sessionID, int(set.Value("id").Number().Raw()), 30, 25, true)
	}
	volumeBefore := historyVolume(e)

	e.DELETE(fmt.Sprintf("/programs/%d/days/%d/assistance/%d",
		programID, dayID, int(created.Value("id").Number().Raw()))).
		Expect().Status(http.StatusNoContent)

	// The session still has them, still labelled assistance, still last.
	reread := e.GET(fmt.Sprintf("/sessions/%d", sessionID)).
		Expect().Status(http.StatusOK).JSON().Object()
	planks := sessionSetsFor(reread, plankID)
	if len(planks) != 2 {
		t.Fatalf("session kept %d plank sets after the plan was removed, want 2", len(planks))
	}
	for _, set := range planks {
		set.Value("kind").String().IsEqual("assistance")
	}
	sets := reread.Value("sets").Array()
	sets.Value(int(sets.Length().Raw()) - 1).Object().
		Value("exerciseId").Number().IsEqual(plankID)

	// ListSessionExerciseWeights is the other query carrying these joins, and it
	// feeds the history list rather than the session detail — so it gets its own
	// assertion, not an inference from the one above.
	summary := historySummary(t, e, sessionID)
	exercises := summary.Value("exercises").Array()
	var listed bool
	for i := 0; i < int(exercises.Length().Raw()); i++ {
		if exercises.Value(i).Object().Value("exerciseName").String().Raw() == "Plank" {
			listed = true
		}
	}
	if !listed {
		t.Fatal("the history row dropped the assistance lift once its plan was removed")
	}

	// And the tonnage is unchanged: removing a plan is not removing work.
	if after := historyVolume(e); after != volumeBefore {
		t.Fatalf("lifetime volume changed when the plan was removed: %v -> %v",
			volumeBefore, after)
	}
}

// Program days are shared, so assistance has to be the thing that is not.
func TestAssistanceIsPrivateToItsOwner(t *testing.T) {
	e := expect(t)
	other := expectAs(t, secondUserToken(t))
	programID, dayID := firstProgramAndDay(e)
	shrugID := exerciseIDByName(t, e, "Barbell Shrug")

	created := addAssistance(t, e, programID, dayID, shrugID, 3, 15, 95)
	assistanceID := int(created.Value("id").Number().Raw())

	// The other lifter sees the same day with no assistance on it.
	day := other.GET(fmt.Sprintf("/programs/%d", programID)).
		Expect().Status(http.StatusOK).JSON().Object().
		Value("days").Array().Value(0).Object()
	day.Value("id").Number().IsEqual(dayID)
	day.Value("assistance").Array().IsEmpty()

	// Their preview is the program's prescription alone.
	preview := other.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("exercises").Array()
	for i := 0; i < int(preview.Length().Raw()); i++ {
		preview.Value(i).Object().Value("kind").String().IsEqual("main")
	}

	// And the row is not addressable by them at all.
	path := fmt.Sprintf("/programs/%d/days/%d/assistance/%d", programID, dayID, assistanceID)
	other.PATCH(path).WithJSON(map[string]any{"sets": 5}).Expect().Status(http.StatusNotFound)
	other.DELETE(path).Expect().Status(http.StatusNotFound)
}

func TestAssistanceValidationAndConflicts(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	faceID := exerciseIDByName(t, e, "Face Pull")

	post := func(body map[string]any) *httpexpect.Response {
		return e.POST(fmt.Sprintf("/programs/%d/days/%d/assistance", programID, dayID)).
			WithJSON(body).Expect()
	}

	post(map[string]any{"exerciseId": faceID, "sets": 0, "reps": 12}).
		Status(http.StatusBadRequest)
	post(map[string]any{"exerciseId": faceID, "sets": 3, "reps": 0}).
		Status(http.StatusBadRequest)
	post(map[string]any{"exerciseId": faceID, "sets": 3, "reps": 12, "weightLb": -5}).
		Status(http.StatusBadRequest)
	post(map[string]any{"exerciseId": 999999, "sets": 3, "reps": 12}).
		Status(http.StatusNotFound)

	addAssistance(t, e, programID, dayID, faceID, 3, 12, 30)
	// One entry per lift per day.
	post(map[string]any{"exerciseId": faceID, "sets": 4, "reps": 12}).
		Status(http.StatusConflict)

	// A day reached through the wrong program is not a day.
	e.POST(fmt.Sprintf("/programs/%d/days/%d/assistance", programID+1000, dayID)).
		WithJSON(map[string]any{"exerciseId": faceID, "sets": 3, "reps": 12}).
		Expect().Status(http.StatusNotFound)
}

// Regression: a lift the program already prescribes on a day must not also be
// addable as assistance on it.
//
// The two tables have no constraint between them, so nothing in the database
// stops it — and the result was not a cosmetic mislabelling. prescribe() emitted
// the lift twice, createSession materialized both, and the second set number 1
// tripped session_sets' UNIQUE (session_id, exercise_id, set_number): starting
// that workout returned 500, and kept doing so until the entry was deleted.
func TestAssistanceRejectsALiftTheDayAlreadyPrescribes(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)

	preview := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("exercises").Array()
	before := int(preview.Length().Raw())
	prescribedID := int(preview.Value(0).Object().Value("exerciseId").Number().Raw())

	e.POST(fmt.Sprintf("/programs/%d/days/%d/assistance", programID, dayID)).
		WithJSON(map[string]any{"exerciseId": prescribedID, "sets": 3, "reps": 8}).
		Expect().Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("already_prescribed")

	// Nothing was added, and the day still starts.
	e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("exercises").Array().Length().IsEqual(before)
	startSession(t, e, dayID)
}

// The same collision approached from the other side: a row that already exists —
// written before the guard above, or created by a program that later adopted the
// lift — must not be able to break the day. prescribe() drops it, so the workout
// still starts and the lift appears once, driven by the engine.
func TestPrescribeSkipsAssistanceTheProgramAlsoPrescribes(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)

	preview := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("exercises").Array()
	before := int(preview.Length().Raw())
	prescribedID := int(preview.Value(0).Object().Value("exerciseId").Number().Raw())

	// Written straight to the database, since the API now refuses to create it —
	// the same reaching past the API that backdateSession does.
	var userID int32
	if err := testPool.QueryRow(context.Background(),
		"SELECT id FROM users WHERE username = $1", primaryUsername).Scan(&userID); err != nil {
		t.Fatalf("look up primary user: %v", err)
	}
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO program_day_assistance
		     (user_id, program_day_id, exercise_id, position, sets, reps, weight_lb)
		 VALUES ($1, $2, $3, 1, 3, 8, 0)`,
		userID, dayID, prescribedID); err != nil {
		t.Fatalf("insert colliding assistance: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			"DELETE FROM program_day_assistance WHERE user_id = $1 AND program_day_id = $2 AND exercise_id = $3",
			userID, dayID, prescribedID)
	})

	// The lift is prescribed once, as the program's own work.
	after := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("exercises").Array()
	after.Length().IsEqual(before)
	seen := 0
	for i := 0; i < int(after.Length().Raw()); i++ {
		ex := after.Value(i).Object()
		if int(ex.Value("exerciseId").Number().Raw()) == prescribedID {
			seen++
			ex.Value("kind").String().IsEqual("main")
		}
	}
	if seen != 1 {
		t.Fatalf("lift %d appears %d times in the prescription, want 1", prescribedID, seen)
	}

	// And the workout still starts, rather than 500ing on the duplicate set.
	session := startSession(t, e, dayID)
	if n := len(sessionSetsFor(session, prescribedID)); n != 5 {
		t.Fatalf("materialized %d sets for the prescribed lift, want its prescribed 5", n)
	}
}

func TestAssistancePatchMergesAndValidates(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	lungeID := exerciseIDByName(t, e, "Walking Lunge")

	created := addAssistance(t, e, programID, dayID, lungeID, 3, 10, 25)
	path := fmt.Sprintf("/programs/%d/days/%d/assistance/%d",
		programID, dayID, int(created.Value("id").Number().Raw()))

	// An omitted field keeps its value.
	updated := e.PATCH(path).WithJSON(map[string]any{"reps": 12}).
		Expect().Status(http.StatusOK).JSON().Object()
	updated.Value("reps").Number().IsEqual(12)
	updated.Value("sets").Number().IsEqual(3)
	updated.Value("weightLb").Number().IsEqual(25)
	updated.Value("exerciseName").String().IsEqual("Walking Lunge")

	e.PATCH(path).WithJSON(map[string]any{"sets": 999}).Expect().Status(http.StatusBadRequest)
	e.PATCH(path).WithJSON(map[string]any{"weightLb": -1}).Expect().Status(http.StatusBadRequest)
}

func TestCustomExerciseLifecycle(t *testing.T) {
	e := expect(t)
	const name = "Copenhagen Plank"

	created := e.POST("/exercises").
		WithJSON(map[string]any{"name": name, "muscleGroup": "core", "equipment": "bodyweight"}).
		Expect().Status(http.StatusCreated).JSON().Object()
	created.Value("isCustom").Boolean().IsTrue()
	created.Value("muscleGroup").String().IsEqual("core")
	id := int(created.Value("id").Number().Raw())

	// Name collisions are refused, case-insensitively, against both the shared
	// catalogue and the caller's own exercises.
	e.POST("/exercises").
		WithJSON(map[string]any{"name": "copenhagen plank", "muscleGroup": "core", "equipment": "bodyweight"}).
		Expect().Status(http.StatusConflict)
	e.POST("/exercises").
		WithJSON(map[string]any{"name": "squat", "muscleGroup": "legs", "equipment": "barbell"}).
		Expect().Status(http.StatusConflict)

	// Bad classifications are 400s, not 500s from the CHECK constraint.
	e.POST("/exercises").
		WithJSON(map[string]any{"name": "Nonsense", "muscleGroup": "spleen", "equipment": "barbell"}).
		Expect().Status(http.StatusBadRequest)
	e.POST("/exercises").
		WithJSON(map[string]any{"name": "  ", "muscleGroup": "core", "equipment": "barbell"}).
		Expect().Status(http.StatusBadRequest)

	// Invisible to everyone else, and undeletable by them.
	other := expectAs(t, secondUserToken(t))
	otherList := other.GET("/exercises").Expect().Status(http.StatusOK).JSON().Array()
	for i := 0; i < int(otherList.Length().Raw()); i++ {
		if otherList.Value(i).Object().Value("name").String().Raw() == name {
			t.Fatalf("%q leaked into another lifter's library", name)
		}
	}
	other.DELETE(fmt.Sprintf("/exercises/%d", id)).Expect().Status(http.StatusNotFound)

	e.DELETE(fmt.Sprintf("/exercises/%d", id)).Expect().Status(http.StatusNoContent)
	e.DELETE(fmt.Sprintf("/exercises/%d", id)).Expect().Status(http.StatusNotFound)
}

// The seeded catalogue is shared, so it is nobody's to delete.
func TestSeededExerciseCannotBeDeleted(t *testing.T) {
	e := expect(t)
	e.DELETE(fmt.Sprintf("/exercises/%d", exerciseIDByName(t, e, "Squat"))).
		Expect().Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("shared_exercise")
}

// A custom exercise that has been performed, or that is on a program day, keeps
// its history and its plan intact rather than being deleted out from under them.
func TestCustomExerciseInUseCannotBeDeleted(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)

	created := e.POST("/exercises").
		WithJSON(map[string]any{
			"name": "Sandbag Carry", "muscleGroup": "core", "equipment": "other",
		}).
		Expect().Status(http.StatusCreated).JSON().Object()
	exerciseID := int(created.Value("id").Number().Raw())

	assistance := addAssistance(t, e, programID, dayID, exerciseID, 2, 20, 60)

	// On a program day: refused, with something to do about it.
	e.DELETE(fmt.Sprintf("/exercises/%d", exerciseID)).
		Expect().Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("exercise_in_use")

	// Perform it, then take it off the day. The logged sets alone still hold it.
	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	for _, set := range sessionSetsFor(session, exerciseID) {
		logSetAt(e, sessionID, int(set.Value("id").Number().Raw()), 20, 60, true)
	}
	e.DELETE(fmt.Sprintf("/programs/%d/days/%d/assistance/%d",
		programID, dayID, int(assistance.Value("id").Number().Raw()))).
		Expect().Status(http.StatusNoContent)

	e.DELETE(fmt.Sprintf("/exercises/%d", exerciseID)).
		Expect().Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("exercise_in_use")

	// Clean up past the API, which deliberately offers no way to do this.
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			"DELETE FROM session_sets WHERE exercise_id = $1", exerciseID)
		_, _ = testPool.Exec(context.Background(),
			"DELETE FROM exercises WHERE id = $1", exerciseID)
	})
}
