package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// exportDoc fetches the export as a JSON object.
func exportDoc(t *testing.T, e *httpexpect.Expect) *httpexpect.Object {
	t.Helper()
	return e.GET("/me/export").Expect().Status(http.StatusOK).JSON().Object()
}

func TestExportRequiresASession(t *testing.T) {
	expectAnon(t).GET("/me/export").Expect().Status(http.StatusUnauthorized)
}

func TestExportCarriesTheProfileAndGym(t *testing.T) {
	e := expect(t)
	doc := exportDoc(t, e)

	doc.Value("formatVersion").Number().IsEqual(1)
	doc.Value("exportedAt").String().NotEmpty()
	doc.HasValue("appVersion", "") // the test server is built with no version

	profile := doc.Value("profile").Object()
	profile.HasValue("username", primaryUsername)
	profile.Value("displayName").String().NotEmpty()
	profile.Value("createdAt").String().NotEmpty()

	// The gym is what the app loads every prescription onto, so an export that
	// omitted it would not describe the numbers in its own sessions.
	gym := doc.Value("gym").Object()
	gym.Value("barWeightLb").Number().Gt(0)
	gym.Value("plates").Array().NotEmpty()
	gym.Value("plates").Array().Value(0).Object().ContainsKey("plateLb").ContainsKey("pairs")
}

// Every array is declared in the spec, so absence has to render as [] rather
// than null — a reader should never have to tell "you have none" apart from
// "this key is missing".
func TestExportRendersEmptyCollectionsAsArrays(t *testing.T) {
	doc := exportDoc(t, expect(t))

	for _, key := range []string{"baselines", "customExercises", "assistance", "sessions"} {
		doc.Value(key).Array() // fails the assertion if it is null
	}
}

func TestExportNamesTheSessionsAndTheirSets(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	setID := int(created.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())

	e.PATCH("/sessions/{sid}/sets/{setid}", sessionID, setID).
		WithJSON(map[string]any{"actualReps": 5, "completed": true}).
		Expect().Status(http.StatusOK)

	sessions := exportDoc(t, e).Value("sessions").Array()
	var found *httpexpect.Object
	for _, value := range sessions.Iter() {
		obj := value.Object()
		if int(obj.Value("id").Number().Raw()) == sessionID {
			found = obj
			break
		}
	}
	if found == nil {
		t.Fatalf("session %d is missing from the export", sessionID)
	}

	found.Value("programName").String().NotEmpty()
	found.Value("programDayName").String().NotEmpty()
	found.Value("performedOn").String().NotEmpty()
	found.ContainsKey("notes")
	found.ContainsKey("bodyweightLb")
	found.ContainsKey("finishedAt")

	sets := found.Value("sets").Array()
	sets.NotEmpty()
	first := sets.Value(0).Object()
	// The name is the point: an export keyed by exercise_id is unreadable
	// anywhere but the database it came from.
	first.Value("exerciseName").String().NotEmpty()
	first.Value("kind").String().IsEqual("main")
	first.Value("setNumber").Number().Ge(1)
	first.Value("targetReps").Number().Ge(1)
	first.ContainsKey("actualReps")
	first.ContainsKey("weightLb")
	first.ContainsKey("completed")
}

// A set that was never logged is part of the record — it says the lifter
// skipped it — so it must survive as actualReps: null rather than be dropped or
// flattened to zero.
func TestExportKeepsUnloggedSetsAsNull(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	sessionID := int(startSession(t, e, dayID).Value("id").Number().Raw())

	for _, value := range exportDoc(t, e).Value("sessions").Array().Iter() {
		obj := value.Object()
		if int(obj.Value("id").Number().Raw()) != sessionID {
			continue
		}
		obj.Value("sets").Array().Value(0).Object().Value("actualReps").IsNull()
		return
	}
	t.Fatalf("session %d is missing from the export", sessionID)
}

func TestExportCarriesCustomExercisesAndBaselines(t *testing.T) {
	e := expect(t)

	created := e.POST("/exercises").
		WithJSON(map[string]any{
			"name":        "Export Test Curl",
			"muscleGroup": "arms",
			"equipment":   "dumbbell",
		}).
		Expect().Status(http.StatusCreated).JSON().Object()
	exerciseID := int(created.Value("id").Number().Raw())
	t.Cleanup(func() {
		e.DELETE("/exercises/{id}", exerciseID).Expect().Status(http.StatusNoContent)
	})

	e.PUT("/me/baselines/{id}", exerciseID).
		WithJSON(map[string]any{"weightLb": 30}).
		Expect().Status(http.StatusNoContent)
	t.Cleanup(func() {
		e.DELETE("/me/baselines/{id}", exerciseID).Expect().Status(http.StatusNoContent)
	})

	doc := exportDoc(t, e)

	var names []string
	for _, value := range doc.Value("customExercises").Array().Iter() {
		names = append(names, value.Object().Value("name").String().Raw())
	}
	if !slices.Contains(names, "Export Test Curl") {
		t.Errorf("custom exercise is missing from the export; got %v", names)
	}

	// Baselines are named, not keyed — ExportBaselines exists rather than
	// reusing ListBaselines precisely for this.
	var baselined []string
	for _, value := range doc.Value("baselines").Array().Iter() {
		obj := value.Object()
		obj.ContainsKey("exerciseName")
		baselined = append(baselined, obj.Value("exerciseName").String().Raw())
	}
	if !slices.Contains(baselined, "Export Test Curl") {
		t.Errorf("baseline is missing from the export; got %v", baselined)
	}
}

func TestExportCarriesAssistanceWork(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)

	exerciseID := int(e.GET("/exercises").Expect().Status(http.StatusOK).
		JSON().Array().Value(0).Object().Value("id").Number().Raw())

	created := e.POST("/programs/{pid}/days/{did}/assistance", programID, dayID).
		WithJSON(map[string]any{
			"exerciseId": exerciseID,
			"sets":       3,
			"reps":       10,
			"weightLb":   25,
		}).
		Expect().Status(http.StatusCreated).JSON().Object()
	assistanceID := int(created.Value("id").Number().Raw())
	t.Cleanup(func() {
		e.DELETE("/programs/{pid}/days/{did}/assistance/{aid}", programID, dayID, assistanceID).
			Expect().Status(http.StatusNoContent)
	})

	assistance := exportDoc(t, e).Value("assistance").Array()
	assistance.NotEmpty()
	first := assistance.Value(0).Object()
	first.Value("programName").String().NotEmpty()
	first.Value("programDayName").String().NotEmpty()
	first.Value("exerciseName").String().NotEmpty()
	first.ContainsKey("repMin")
	first.ContainsKey("repMax")
}

// The point of the header is that a browser saves the file instead of painting
// a year of training into a tab.
func TestExportIsServedAsADownload(t *testing.T) {
	resp := expect(t).GET("/me/export").Expect().Status(http.StatusOK)

	resp.Header("Content-Type").Contains("application/json")
	disposition := resp.Header("Content-Disposition")
	disposition.Contains("attachment")
	disposition.Contains(primaryUsername)
	disposition.Contains(".json")
}

// An export is a snapshot of an account, and caching one would hand a stale
// copy to the next request — or, worse, leave it in a shared cache.
func TestExportIsNotCached(t *testing.T) {
	resp := expect(t).GET("/me/export").Expect().Status(http.StatusOK)

	resp.Header("Cache-Control").Contains("no-store")
	resp.Header("ETag").IsEmpty()
}

// Login sessions are live credentials and the avatar is a binary blob. Neither
// belongs in a file that lands in a downloads folder.
func TestExportOmitsCredentialsAndBinaries(t *testing.T) {
	body := strings.ToLower(expect(t).GET("/me/export").Expect().Status(http.StatusOK).Body().Raw())

	for _, forbidden := range []string{
		"token", "passwordhash", "password_hash",
		"usersessions", "user_sessions", "avatarimage", "avatardata",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the export body mentions %q", forbidden)
		}
	}
}
