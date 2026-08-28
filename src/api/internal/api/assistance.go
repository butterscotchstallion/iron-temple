package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Assistance work: the exercises a lifter attaches to a program day for
// themselves.
//
// The programs are shared and seeded, and these handlers never touch them. What
// they write is an overlay keyed on (user_id, program_day_id), so two accounts
// looking at the same Workout A see the same three barbell lifts and their own
// assistance under it. That separation is what lets the plan be edited freely
// without the progression engine or the Racked recap losing their footing: the
// engine reads a prescription nobody can change, and the recap reads performed
// sets, which are a fact either way.
//
// Every handler here resolves the day through its program and scopes the row to
// the caller. A row belonging to someone else 404s rather than 403s — the same
// rule sessions.sql applies, for the same reason: learning that an id is valid
// is already a leak.

// Bounds on an assistance prescription, mirroring openapi.yaml. They exist to
// keep a typo from materializing a thousand sets into a session, not because any
// particular number is meaningful.
const (
	maxAssistanceSets = 20
	maxAssistanceReps = 100
)

type createAssistanceRequest struct {
	ExerciseID int32    `json:"exerciseId"`
	Sets       int32    `json:"sets"`
	Reps       int32    `json:"reps"`
	WeightLb   *float64 `json:"weightLb"`
	// RepMin and RepMax put the lift on double progression. Both or neither —
	// "8 to null" is not a range, it is a mistake, and the database says so too.
	RepMin *int32 `json:"repMin"`
	RepMax *int32 `json:"repMax"`
}

// validRepRange reports whether a proposed range is usable, and says why if not.
// Both bounds or neither, in range, and running the right way round. Mirrors
// program_day_assistance_rep_range_ck, so the constraint is a backstop rather
// than the thing producing the error a lifter reads.
func validRepRange(repMin, repMax *int32) (string, bool) {
	if repMin == nil && repMax == nil {
		return "", true
	}
	if repMin == nil || repMax == nil {
		return "repMin and repMax must be given together, or neither", false
	}
	if *repMin < 1 || *repMin > maxAssistanceReps || *repMax < 1 || *repMax > maxAssistanceReps {
		return "repMin and repMax must be between 1 and 100", false
	}
	if *repMax < *repMin {
		return "repMax must be at least repMin", false
	}
	return "", true
}

func (s *Server) addAssistance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day, ok := s.programDay(w, r)
	if !ok {
		return
	}

	var req createAssistanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.ExerciseID <= 0 {
		badRequest(w, "exerciseId is required")
		return
	}
	if req.Sets < 1 || req.Sets > maxAssistanceSets {
		badRequest(w, "sets must be between 1 and 20")
		return
	}
	if req.Reps < 1 || req.Reps > maxAssistanceReps {
		badRequest(w, "reps must be between 1 and 100")
		return
	}
	if msg, ok := validRepRange(req.RepMin, req.RepMax); !ok {
		badRequest(w, msg)
		return
	}
	// Absent means bodyweight, which is the common case for assistance — dips,
	// chin-ups, planks — so it is a default rather than a required field.
	weight := 0.0
	if req.WeightLb != nil {
		if *req.WeightLb < 0 {
			badRequest(w, "weightLb must be a non-negative number")
			return
		}
		weight = *req.WeightLb
	}

	userID := userFrom(ctx).ID

	// Resolved through the scoped GetExercise, so another lifter's custom
	// exercise cannot be attached to a day — and cannot be probed for existence
	// by watching which ids 404.
	ex, err := s.q.GetExercise(ctx, store.GetExerciseParams{ID: req.ExerciseID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "exercise not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	// A lift the program already prescribes on this day cannot also be assistance
	// on it, and the database cannot say so: the UNIQUE on program_day_assistance
	// covers that table alone, and the prescription lives in another one.
	//
	// Without this check the day is not merely mislabelled, it is unusable.
	// prescribe() would return the lift twice — once from the prescription, once
	// from assistance — and createSession materializes both, so the second set
	// number 1 collides with the first on session_sets' UNIQUE (session_id,
	// exercise_id, set_number). Every attempt to start that workout 500s, and
	// keeps 500ing until the assistance entry is deleted.
	//
	// It would be wrong even if it worked. is_assistance is derived from whether
	// a program_day_exercises row exists, so the duplicate's sets would come back
	// labelled "main" and ordered by the prescription's position — the engine's
	// weight and the carried-forward weight fighting over the same bar.
	prescribed, err := s.q.ListPrescriptionsByDay(ctx, day.ID)
	if err != nil {
		internalError(w)
		return
	}
	for _, p := range prescribed {
		if p.ExerciseID == ex.ID {
			conflict(w, "already_prescribed",
				"this day already prescribes that lift — assistance is for work the program doesn't cover")
			return
		}
	}

	created, err := s.q.CreateAssistance(ctx, store.CreateAssistanceParams{
		UserID:       userID,
		ProgramDayID: day.ID,
		ExerciseID:   ex.ID,
		Sets:         req.Sets,
		Reps:         req.Reps,
		WeightLb:     floatToNumeric(weight),
		RepMin:       req.RepMin,
		RepMax:       req.RepMax,
	})
	// One entry per lift per day: adding dips twice is an edit of the first
	// entry, not a second row. The unique constraint decides that, so a
	// double-tap lands here rather than as a 500.
	if isUniqueViolation(err) {
		conflict(w, "duplicate_assistance", "that exercise is already on this day")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusCreated, programDayAssistanceDTO{
		ID:           created.ID,
		ExerciseID:   created.ExerciseID,
		ExerciseName: ex.Name,
		Position:     created.Position,
		Sets:         created.Sets,
		Reps:         created.Reps,
		WeightLb:     numericToFloat(created.WeightLb),
		RepMin:       created.RepMin,
		RepMax:       created.RepMax,
	})
}

type updateAssistanceRequest struct {
	Sets     *int32   `json:"sets"`
	Reps     *int32   `json:"reps"`
	WeightLb *float64 `json:"weightLb"`
	// RepMin and RepMax are decoded from the raw body rather than taken from
	// here, because null is meaningful for them and a *int32 renders absent and
	// null identically. Turning the range off is how a lifter puts a lift back on
	// carry-forward, so it has to be sayable. See updateAssistance.
}

func (s *Server) updateAssistance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day, ok := s.programDay(w, r)
	if !ok {
		return
	}
	id, ok := idParam(r, "assistanceId")
	if !ok {
		notFound(w, "assistance not found")
		return
	}

	// Read twice: once into the struct for the fields where a pointer is enough,
	// and once as raw messages for repMin/repMax, where null is an instruction
	// rather than a synonym for absent — the same distinction updateSession draws
	// for bodyweightLb. Sending null turns the rep range off and puts the lift
	// back on carrying its weight forward.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	var req updateAssistanceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	userID := userFrom(ctx).ID
	current, err := s.q.GetAssistance(ctx, store.GetAssistanceParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && current.ProgramDayID != day.ID) {
		notFound(w, "assistance not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	// Merge the patch over current values, the same shape as updateSessionSet.
	// No field here is nullable, so a pointer is enough to tell absent from set
	// and the raw-message decode that one needs is unnecessary.
	params := store.UpdateAssistanceParams{
		ID:       id,
		UserID:   userID,
		Sets:     current.Sets,
		Reps:     current.Reps,
		WeightLb: current.WeightLb,
		RepMin:   current.RepMin,
		RepMax:   current.RepMax,
	}
	if req.Sets != nil {
		if *req.Sets < 1 || *req.Sets > maxAssistanceSets {
			badRequest(w, "sets must be between 1 and 20")
			return
		}
		params.Sets = *req.Sets
	}
	if req.Reps != nil {
		if *req.Reps < 1 || *req.Reps > maxAssistanceReps {
			badRequest(w, "reps must be between 1 and 100")
			return
		}
		params.Reps = *req.Reps
	}
	if req.WeightLb != nil {
		if *req.WeightLb < 0 {
			badRequest(w, "weightLb must be a non-negative number")
			return
		}
		params.WeightLb = floatToNumeric(*req.WeightLb)
	}

	// The range is validated as a pair after the merge, not field by field: a
	// PATCH that raises repMax alone still has to end up with a range that runs
	// the right way round, and only the merged values know that.
	if v, ok := raw["repMin"]; ok {
		params.RepMin = nil
		if string(v) != "null" {
			var n int32
			if err := json.Unmarshal(v, &n); err != nil {
				badRequest(w, "repMin must be an integer or null")
				return
			}
			params.RepMin = &n
		}
	}
	if v, ok := raw["repMax"]; ok {
		params.RepMax = nil
		if string(v) != "null" {
			var n int32
			if err := json.Unmarshal(v, &n); err != nil {
				badRequest(w, "repMax must be an integer or null")
				return
			}
			params.RepMax = &n
		}
	}
	if msg, ok := validRepRange(params.RepMin, params.RepMax); !ok {
		badRequest(w, msg)
		return
	}

	updated, err := s.q.UpdateAssistance(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "assistance not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, programDayAssistanceDTO{
		ID:           updated.ID,
		ExerciseID:   updated.ExerciseID,
		ExerciseName: current.ExerciseName,
		Position:     updated.Position,
		Sets:         updated.Sets,
		Reps:         updated.Reps,
		WeightLb:     numericToFloat(updated.WeightLb),
		RepMin:       updated.RepMin,
		RepMax:       updated.RepMax,
	})
}

// removeAssistance drops the plan and leaves the performances alone. Sets
// already logged against the exercise stay in the sessions that recorded them —
// ListSessionSets orders them with a fallback precisely so they stay visible
// after the row they were prescribed from is gone.
func (s *Server) removeAssistance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day, ok := s.programDay(w, r)
	if !ok {
		return
	}
	id, ok := idParam(r, "assistanceId")
	if !ok {
		notFound(w, "assistance not found")
		return
	}

	userID := userFrom(ctx).ID
	current, err := s.q.GetAssistance(ctx, store.GetAssistanceParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && current.ProgramDayID != day.ID) {
		notFound(w, "assistance not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	n, err := s.q.DeleteAssistance(ctx, store.DeleteAssistanceParams{ID: id, UserID: userID})
	if err != nil {
		internalError(w)
		return
	}
	if n == 0 {
		notFound(w, "assistance not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// programDay resolves the {programId}/{dayId} pair every assistance route is
// mounted under, confirming the day belongs to the program named in the path. A
// day reached through the wrong program is a 404: the URL asserts a relationship
// that does not hold, and honouring it would make the same row addressable under
// every program id there is.
//
// Writes the 404 itself and reports false, so callers can `if !ok { return }`.
func (s *Server) programDay(w http.ResponseWriter, r *http.Request) (store.ProgramDay, bool) {
	programID, ok := idParam(r, "programId")
	if !ok {
		notFound(w, "program day not found")
		return store.ProgramDay{}, false
	}
	dayID, ok := idParam(r, "dayId")
	if !ok {
		notFound(w, "program day not found")
		return store.ProgramDay{}, false
	}

	day, err := s.q.GetProgramDay(r.Context(), dayID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && day.ProgramID != programID) {
		notFound(w, "program day not found")
		return store.ProgramDay{}, false
	}
	if err != nil {
		internalError(w)
		return store.ProgramDay{}, false
	}
	return day, true
}
