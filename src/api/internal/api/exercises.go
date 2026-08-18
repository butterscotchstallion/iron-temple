package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// The exercise library: the seeded catalogue every account shares, plus the
// movements a lifter added for themselves.
//
// Sharing is the default and ownership is the exception, which is the opposite
// of how sessions work, so the scoping rule is worth stating plainly: a row with
// created_by_user_id NULL is visible to everyone and deletable by no one, and a
// row with an owner is visible and deletable only to them. Every query in
// exercises.sql carries that filter; these handlers never widen it.

// maxExerciseNameLen bounds a custom exercise's name. It matches the maxLength
// in openapi.yaml — the library renders names in a single line, and the column
// is TEXT, so this is a UI constraint rather than a storage one.
const maxExerciseNameLen = 80

// The classification a custom exercise must pick from. These mirror the CHECK
// constraints added by migration 0009; validating here turns a bad value into a
// 400 that names the field instead of a 500 from the database.
var (
	muscleGroups = map[string]bool{
		"chest": true, "back": true, "legs": true,
		"shoulders": true, "arms": true, "core": true, "other": true,
	}
	equipmentKinds = map[string]bool{
		"barbell": true, "dumbbell": true, "machine": true,
		"cable": true, "bodyweight": true, "other": true,
	}
)

func (s *Server) listExercises(w http.ResponseWriter, r *http.Request) {
	// scope=performed is what the Progress page asks for: the lifts with a
	// history to chart, rather than the whole library with most of it reading
	// "no sessions yet". An unrecognised value is a 400 and not a silent "all",
	// so a typo in a caller shows up as a typo.
	performedOnly := false
	switch scope := r.URL.Query().Get("scope"); scope {
	case "", "all":
	case "performed":
		performedOnly = true
	default:
		badRequest(w, "scope must be all or performed")
		return
	}

	rows, err := s.q.ListExercises(r.Context(), store.ListExercisesParams{
		UserID: userFrom(r.Context()).ID, PerformedOnly: performedOnly,
	})
	if err != nil {
		internalError(w)
		return
	}
	out := make([]exerciseDTO, 0, len(rows))
	for _, e := range rows {
		out = append(out, exerciseDTO{
			ID:          e.ID,
			Name:        e.Name,
			MuscleGroup: e.MuscleGroup,
			Equipment:   e.Equipment,
			IsAccessory: e.IsAccessory,
			IsCustom:    e.IsCustom,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type createExerciseRequest struct {
	Name        string `json:"name"`
	MuscleGroup string `json:"muscleGroup"`
	Equipment   string `json:"equipment"`
}

func (s *Server) createExercise(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createExerciseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > maxExerciseNameLen {
		badRequest(w, "name must be between 1 and 80 characters")
		return
	}
	if !muscleGroups[req.MuscleGroup] {
		badRequest(w, "muscleGroup must be one of chest, back, legs, shoulders, arms, core, other")
		return
	}
	if !equipmentKinds[req.Equipment] {
		badRequest(w, "equipment must be one of barbell, dumbbell, machine, cable, bodyweight, other")
		return
	}

	userID := userFrom(ctx).ID

	// Checked here as well as enforced by the partial unique indexes, because
	// the two answer different questions. The index stops the write; this says
	// which write it stopped, and lets the UI put the message next to the field.
	// The index remains the guarantee — a race past this check still fails, and
	// fails safely.
	conflicts, err := s.q.CountExerciseNameConflicts(ctx, store.CountExerciseNameConflictsParams{
		Name: name, UserID: userID,
	})
	if err != nil {
		internalError(w)
		return
	}
	if conflicts > 0 {
		conflict(w, "duplicate_name", "an exercise with that name already exists")
		return
	}

	created, err := s.q.CreateExercise(ctx, store.CreateExerciseParams{
		Name:        name,
		MuscleGroup: req.MuscleGroup,
		Equipment:   req.Equipment,
		UserID:      userID,
	})
	if isUniqueViolation(err) {
		conflict(w, "duplicate_name", "an exercise with that name already exists")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusCreated, exerciseDTO{
		ID:          created.ID,
		Name:        created.Name,
		MuscleGroup: created.MuscleGroup,
		Equipment:   created.Equipment,
		IsAccessory: created.IsAccessory,
		IsCustom:    created.IsCustom,
	})
}

// deleteExercise removes one of the caller's own movements from the library.
//
// The two 409s are the interesting part. A seeded exercise is refused because it
// is not the caller's to remove — programs prescribe it and other accounts see
// it. A custom one with logged sets is refused because deleting it would take a
// lift out of a finished session, and a finished session is a record; the
// database would refuse the delete anyway, since session_sets.exercise_id has no
// ON DELETE clause, so this turns a foreign-key error into an explanation. An
// exercise merely sitting on a program day would cascade cleanly, but silently
// rewriting someone's workout from the library screen is a surprise, so that is
// refused too — with the day named, so there is something to do about it.
func (s *Server) deleteExercise(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "exerciseId")
	if !ok {
		notFound(w, "exercise not found")
		return
	}
	ctx := r.Context()
	userID := userFrom(ctx).ID

	ex, err := s.q.GetExercise(ctx, store.GetExerciseParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "exercise not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}
	if !ex.IsCustom {
		conflict(w, "shared_exercise", "the seeded exercises can't be deleted")
		return
	}

	uses, err := s.q.CountExerciseUses(ctx, id)
	if err != nil {
		internalError(w)
		return
	}
	if uses.LoggedSets > 0 {
		conflict(w, "exercise_in_use", "this exercise has logged sets, so it can't be deleted")
		return
	}
	if uses.AssistanceEntries > 0 {
		conflict(w, "exercise_in_use", "remove this exercise from your programs before deleting it")
		return
	}

	n, err := s.q.DeleteExercise(ctx, store.DeleteExerciseParams{ID: id, UserID: userID})
	if err != nil {
		internalError(w)
		return
	}
	if n == 0 {
		notFound(w, "exercise not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getExerciseHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "exerciseId")
	if !ok {
		notFound(w, "exercise not found")
		return
	}
	rows, err := s.q.ListExerciseHistory(r.Context(), store.ListExerciseHistoryParams{
		ExerciseID: id, UserID: userFrom(r.Context()).ID,
	})
	if err != nil {
		internalError(w)
		return
	}
	out := make([]exerciseHistoryPointDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, exerciseHistoryPointDTO{
			PerformedOn: dateToString(row.PerformedOn),
			WeightLb:    numericToFloat(row.WeightLb),
			Reps:        row.Reps,
			Completed:   row.Completed,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
