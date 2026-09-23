package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// The days of a program a lifter owns, and the lifts prescribed on them.
//
// Everything here goes through ownedProgramDay, so the seeded catalogue and
// other people's programs are both 404 — see program_editor.go for why that
// falls out of the owner column rather than out of a check.
//
// The one exception is the weekday, which is editable on ANY program the caller
// can see and has been since 0003. See updateProgramDay.

// Bounds on a prescription. Shared with assistance, because they are the same
// kind of claim about the same kind of thing: a block of sets and reps a lifter
// typed in, bounded so a typo cannot materialize a thousand sets into a session.
const (
	maxPrescribedSets = maxAssistanceSets
	maxPrescribedReps = maxAssistanceReps
	// NUMERIC(6,2) would accept 9999.99 and then fail on anything larger with a
	// 500. This is the same ceiling the starting-weight input on the program
	// screen already uses, so the client and the server refuse the same things.
	maxStartingWeightLb = 2000
)

type createProgramDayRequest struct {
	Name    string `json:"name"`
	Weekday *int32 `json:"weekday"`
}

func (s *Server) addProgramDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	program, ok := s.ownedProgram(w, r)
	if !ok {
		return
	}

	var req createProgramDayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > maxProgramNameLen {
		badRequest(w, "name must be between 1 and 80 characters")
		return
	}
	if !validWeekday(w, req.Weekday) {
		return
	}

	dayID, err := s.q.CreateProgramDay(ctx, store.CreateProgramDayParams{
		ProgramID: program.ID, Name: name,
	})
	if isUniqueViolation(err) {
		conflict(w, "duplicate_day", "this program already has a day with that name")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	if req.Weekday != nil {
		if _, err := s.q.UpdateProgramDay(ctx, store.UpdateProgramDayParams{
			ID: dayID, Weekday: req.Weekday,
		}); err != nil {
			internalError(w)
			return
		}
	}

	s.writeProgram(w, r, program.ID, http.StatusCreated)
}

type updateProgramDayRequest struct {
	Name    *string `json:"name"`
	Weekday *int32  `json:"weekday"`
}

// updateProgramDay renames a day and schedules it.
//
// # Two fields with two different authorization rules, deliberately
//
// The weekday is editable on ANY program the caller can see, which is what it
// has been since 0003 and what the program screen's weekday picker depends on:
// the seeded programs have no owner, so owner-scoping this outright would remove
// the ability to schedule StrongLifts at all. The name is editable only by the
// program's owner, because it is part of the program rather than part of the
// lifter's week.
//
// That split is written down because it looks like a bug otherwise. It is also
// only half a fix for a real one: program_days.weekday is a SHARED column with
// no user in it, so on a multi-lifter install one lifter rescheduling Workout A
// reschedules it for everybody. activity.sql's SetProgramDayWeekdayIfUnset
// documents that at length and refuses to overwrite for exactly this reason —
// which leaves the generated-activity runner more careful with other people's
// schedules than this endpoint is. The real answer is a per-user schedule table
// shaped like program_day_assistance; until then, custom programs at least do
// not add a second way to hit it.
func (s *Server) updateProgramDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Resolves the day and confirms the caller may SEE the program. Ownership is
	// checked below, and only for the fields that need it.
	day, ok := s.programDay(w, r)
	if !ok {
		return
	}

	// Read twice: once into the struct, and once as raw messages, because a
	// weekday of null MEANS unscheduled rather than "leave it alone" — the same
	// distinction updateAssistance draws for repMin/repMax. Before this endpoint
	// took a name there was only one field and the difference did not arise; with
	// two, decoding into a bare *int32 would let PATCH {"name":"Squat Day"}
	// silently clear the schedule.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	var req updateProgramDayRequest
	if err := json.Unmarshal(body, &req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	// Absent leaves the schedule as it is; present-and-null clears it.
	weekday := day.Weekday
	if _, given := raw["weekday"]; given {
		if !validWeekday(w, req.Weekday) {
			return
		}
		weekday = req.Weekday
	}

	var name *string
	if req.Name != nil {
		// Renaming is a change to the program, so it needs the program's owner.
		if !s.ownsProgram(w, r, day.ProgramID) {
			return
		}
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" || len(trimmed) > maxProgramNameLen {
			badRequest(w, "name must be between 1 and 80 characters")
			return
		}
		name = &trimmed
	}

	n, err := s.q.UpdateProgramDay(ctx, store.UpdateProgramDayParams{
		ID: day.ID, Name: name, Weekday: weekday,
	})
	if isUniqueViolation(err) {
		conflict(w, "duplicate_day", "this program already has a day with that name")
		return
	}
	if err != nil {
		internalError(w)
		return
	}
	if n == 0 {
		notFound(w, "program day not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeProgramDay takes a day out of a program: deleted if nobody has trained
// it, archived if somebody has.
//
// The split is not a policy choice, it is what the schema allows.
// sessions.program_day_id has no ON DELETE clause, so a day with a session
// against it cannot be deleted — and making that key cascade would mean removing
// a day from a program destroyed the workouts performed on it. Archiving keeps
// the row resolvable so the history page, the recap and every Racked figure can
// still read the day's name, while the program stops offering it.
func (s *Server) removeProgramDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day, ok := s.ownedProgramDay(w, r)
	if !ok {
		return
	}

	uses, err := s.q.CountDayUses(ctx, day.ID)
	if err != nil {
		internalError(w)
		return
	}

	// Archived rather than deleted when there is history behind it. Note this
	// counts EVERY lifter's sessions, not just the owner's: on a shared program
	// a follower's workout is exactly as much a reason not to delete the row.
	if uses > 0 {
		if _, err := s.q.ArchiveProgramDay(ctx, day.ID); err != nil {
			internalError(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if _, err := s.q.DeleteProgramDay(ctx, day.ID); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Prescriptions
// ---------------------------------------------------------------------------

type createPrescriptionRequest struct {
	ExerciseID       int32    `json:"exerciseId"`
	Sets             int32    `json:"sets"`
	Reps             int32    `json:"reps"`
	StartingWeightLb *float64 `json:"startingWeightLb"`
}

func (s *Server) addPrescription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day, ok := s.ownedProgramDay(w, r)
	if !ok {
		return
	}

	var req createPrescriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	weight, msg, valid := validPrescription(req.Sets, req.Reps, req.StartingWeightLb)
	if !valid {
		badRequest(w, msg)
		return
	}

	// The movement has to exist and be one this lifter can use — their own or the
	// install's. listExercises applies the same rule, so this refuses ids that
	// screen would never have offered.
	if _, err := s.q.GetExercise(ctx, store.GetExerciseParams{
		ID: req.ExerciseID, UserID: userFrom(ctx).ID,
	}); errors.Is(err, pgx.ErrNoRows) {
		badRequest(w, "exerciseId must be an exercise you can use")
		return
	} else if err != nil {
		internalError(w)
		return
	}

	id, err := s.q.CreatePrescription(ctx, store.CreatePrescriptionParams{
		ProgramDayID:     day.ID,
		ExerciseID:       req.ExerciseID,
		Sets:             req.Sets,
		Reps:             req.Reps,
		StartingWeightLb: floatToNumeric(weight),
	})
	if isUniqueViolation(err) {
		// UNIQUE (program_day_id, exercise_id). "3x8 dips and also 3x10 dips" is
		// one entry with the sets edited rather than two rows to reconcile — the
		// rule program_day_assistance states and this table has had since 0001.
		conflict(w, "duplicate_exercise", "that lift is already on this day")
		return
	}
	if err != nil {
		internalError(w)
		return
	}
	_ = id

	s.writeProgram(w, r, day.ProgramID, http.StatusCreated)
}

type updatePrescriptionRequest struct {
	Sets             *int32   `json:"sets"`
	Reps             *int32   `json:"reps"`
	StartingWeightLb *float64 `json:"startingWeightLb"`
}

// updatePrescription changes the sets, reps or starting weight of a lift.
//
// Not the exercise: swapping the movement is a remove plus an add, because
// starting_weight_lb means something different for a different lift and silently
// carrying it across would put a squat's opener on a lateral raise.
func (s *Server) updatePrescription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day, ok := s.ownedProgramDay(w, r)
	if !ok {
		return
	}
	current, ok := s.prescriptionOnDay(w, r, day.ID)
	if !ok {
		return
	}

	var req updatePrescriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	// Merge the patch over current values, the same shape as updateAssistance.
	// No field here is nullable, so a pointer tells absent from set and the
	// raw-message decode that endpoint needs is unnecessary.
	sets, reps := current.Sets, current.Reps
	weight := numericToFloat(current.StartingWeightLb)
	if req.Sets != nil {
		sets = *req.Sets
	}
	if req.Reps != nil {
		reps = *req.Reps
	}
	if req.StartingWeightLb != nil {
		weight = *req.StartingWeightLb
	}
	resolved, msg, valid := validPrescription(sets, reps, &weight)
	if !valid {
		badRequest(w, msg)
		return
	}

	n, err := s.q.UpdatePrescription(ctx, store.UpdatePrescriptionParams{
		ID: current.ID, Sets: sets, Reps: reps,
		StartingWeightLb: floatToNumeric(resolved),
	})
	if err != nil {
		internalError(w)
		return
	}
	if n == 0 {
		notFound(w, "exercise not found on this day")
		return
	}

	s.writeProgram(w, r, day.ProgramID, http.StatusOK)
}

func (s *Server) removePrescription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day, ok := s.ownedProgramDay(w, r)
	if !ok {
		return
	}
	current, ok := s.prescriptionOnDay(w, r, day.ID)
	if !ok {
		return
	}

	// No confirmation and no archiving, because this loses a plan and never a
	// performance: session_sets references the movement, not this row, so every
	// set ever logged for the lift stays and keeps counting. Re-adding it is one
	// tap away, which is why removeAssistance asks for no confirmation either.
	if _, err := s.q.DeletePrescription(ctx, current.ID); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Reordering
// ---------------------------------------------------------------------------

type reorderRequest struct {
	IDs []int32 `json:"ids"`
}

// reorderPrescriptions rewrites the order of a day's lifts.
//
// Takes the COMPLETE ordered list rather than "move this one to position N",
// which makes it idempotent and immune to a lost update: two clients sending
// their view of the order leave the second one's, rather than interleaving into
// something neither asked for.
func (s *Server) reorderPrescriptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day, ok := s.ownedProgramDay(w, r)
	if !ok {
		return
	}

	ids, ok := decodeReorder(w, r)
	if !ok {
		return
	}

	// Park, then place — see the comment on ParkPrescriptionPositions for why a
	// single UPDATE cannot do this.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	if err := qtx.ParkPrescriptionPositions(ctx, day.ID); err != nil {
		internalError(w)
		return
	}
	moved, err := qtx.ReorderPrescriptions(ctx, store.ReorderPrescriptionsParams{
		ProgramDayID: day.ID, Ids: ids,
	})
	if err != nil {
		internalError(w)
		return
	}

	// Every live row must have been named. An id list missing one would otherwise
	// leave it parked at max+n for ever — an order nothing can read and nobody is
	// told about, which is worse than a rejected request.
	pres, err := qtx.ListPrescriptionsByDay(ctx, day.ID)
	if err != nil {
		internalError(w)
		return
	}
	if int(moved) != len(ids) || len(ids) != len(pres) {
		badRequest(w, "ids must name every exercise on this day, exactly once")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}
	s.writeProgram(w, r, day.ProgramID, http.StatusOK)
}

func (s *Server) reorderProgramDays(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	program, ok := s.ownedProgram(w, r)
	if !ok {
		return
	}

	ids, ok := decodeReorder(w, r)
	if !ok {
		return
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	if err := qtx.ParkProgramDayPositions(ctx, program.ID); err != nil {
		internalError(w)
		return
	}
	moved, err := qtx.ReorderProgramDays(ctx, store.ReorderProgramDaysParams{
		ProgramID: program.ID, Ids: ids,
	})
	if err != nil {
		internalError(w)
		return
	}
	days, err := qtx.ListProgramDays(ctx, program.ID)
	if err != nil {
		internalError(w)
		return
	}
	if int(moved) != len(ids) || len(ids) != len(days) {
		badRequest(w, "ids must name every day of this program, exactly once")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}
	s.writeProgram(w, r, program.ID, http.StatusOK)
}

// decodeReorder reads and sanity-checks an id list, writing the 400 itself.
//
// The duplicate check is here rather than left to the row count, because
// `[7, 7]` against a two-row day moves one row and reports one — which would
// pass a bare count comparison while leaving the other parked.
func decodeReorder(w http.ResponseWriter, r *http.Request) ([]int32, bool) {
	var req reorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return nil, false
	}
	if len(req.IDs) == 0 {
		badRequest(w, "ids must not be empty")
		return nil, false
	}
	seen := make(map[int32]bool, len(req.IDs))
	for _, id := range req.IDs {
		if seen[id] {
			badRequest(w, "ids must not repeat")
			return nil, false
		}
		seen[id] = true
	}
	return req.IDs, true
}

// ---------------------------------------------------------------------------
// Shared resolution and validation
// ---------------------------------------------------------------------------

// ownedProgramDay is programDay() with the ownership check folded in: the day
// belongs to the program in the path, the caller can see it, and it is theirs.
func (s *Server) ownedProgramDay(w http.ResponseWriter, r *http.Request) (store.ProgramDay, bool) {
	day, ok := s.programDay(w, r)
	if !ok {
		return store.ProgramDay{}, false
	}
	if !s.ownsProgram(w, r, day.ProgramID) {
		return store.ProgramDay{}, false
	}
	return day, true
}

// ownsProgram reports whether the caller owns a program, writing the 404 itself.
//
// A seeded program has NULL for an owner, so this is false for everybody, which
// is what keeps the install's catalogue out of the editor without a rule of its
// own. 404 rather than 403 throughout: an id that answers differently depending
// on who owns it is an id that can be enumerated.
func (s *Server) ownsProgram(w http.ResponseWriter, r *http.Request, programID int32) bool {
	ctx := r.Context()
	userID := userFrom(ctx).ID
	program, err := s.q.GetProgram(ctx, store.GetProgramParams{ID: programID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "program day not found")
		return false
	}
	if err != nil {
		internalError(w)
		return false
	}
	if program.CreatedByUserID == nil || *program.CreatedByUserID != userID {
		notFound(w, "program day not found")
		return false
	}
	return true
}

// prescriptionOnDay resolves {prescriptionId} and confirms it is on this day.
//
// A prescription reached through the wrong day is a 404 for the reason
// programDay() gives about days and programs: the URL asserts a relationship
// that does not hold, and honouring it would make the row addressable under
// every day id there is.
func (s *Server) prescriptionOnDay(
	w http.ResponseWriter, r *http.Request, dayID int32,
) (store.ProgramDayExercise, bool) {
	id, ok := idParam(r, "prescriptionId")
	if !ok {
		notFound(w, "exercise not found on this day")
		return store.ProgramDayExercise{}, false
	}
	row, err := s.q.GetPrescription(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.ProgramDayID != dayID) {
		notFound(w, "exercise not found on this day")
		return store.ProgramDayExercise{}, false
	}
	if err != nil {
		internalError(w)
		return store.ProgramDayExercise{}, false
	}
	return row, true
}

// validWeekday checks a schedule slot, writing the 400 itself. nil is valid and
// means unscheduled.
func validWeekday(w http.ResponseWriter, weekday *int32) bool {
	if weekday != nil && (*weekday < 0 || *weekday > 6) {
		badRequest(w, "weekday must be between 0 and 6")
		return false
	}
	return true
}

// validPrescription checks the numbers a prescribed lift is made of and resolves
// its starting weight. The sibling of validAssistancePrescription, and bounded
// the same way for the same reason.
//
// An absent weight means 0, which is right for bodyweight work and harmless
// otherwise: starting_weight_lb is only ever consulted for a lift with no
// history, so it decides the first session and nothing after it.
func validPrescription(sets, reps int32, startingWeightLb *float64) (float64, string, bool) {
	if sets < 1 || sets > maxPrescribedSets {
		return 0, "sets must be between 1 and 20", false
	}
	if reps < 1 || reps > maxPrescribedReps {
		return 0, "reps must be between 1 and 100", false
	}
	if startingWeightLb == nil {
		return 0, "", true
	}
	if *startingWeightLb < 0 || *startingWeightLb > maxStartingWeightLb {
		return 0, "startingWeightLb must be between 0 and 2000", false
	}
	return *startingWeightLb, "", true
}
