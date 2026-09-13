package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Adding assistance from inside a workout.
//
// This is the one write in the API that reaches both a session and a program,
// and it exists because the two paths either side of it each stop short of the
// lifter standing at the rack:
//
//   - POST /sessions/{id}/sets appends a set to a lift ALREADY in the session.
//     AppendSessionSet copies weight and reps from that lift's last set, so with
//     no existing set there is no source row. It adds a set, not an exercise.
//   - POST /programs/{id}/days/{id}/assistance edits the overlay, and says
//     plainly that it takes effect on the NEXT session — a session in progress
//     keeps the sets it was created with, because sessions materialise up front.
//
// So neither one covers "I have decided to do some curls, now". This does both
// halves in one transaction. Doing them in two calls would leave a window where
// the program has gained a lift the workout in front of the lifter has not, and
// nothing in the app could explain that to them.
type addSessionAssistanceRequest struct {
	ExerciseID int32    `json:"exerciseId"`
	Sets       int32    `json:"sets"`
	Reps       int32    `json:"reps"`
	WeightLb   *float64 `json:"weightLb"`
}

func (s *Server) addSessionAssistance(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := idParam(r, "sessionId")
	if !ok {
		notFound(w, "session not found")
		return
	}

	var req addSessionAssistanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.ExerciseID <= 0 {
		badRequest(w, "exerciseId is required")
		return
	}
	// Shared with addAssistance rather than restated, so the two entry points
	// cannot drift about what a usable prescription is.
	weight, msg, ok := validAssistancePrescription(req.Sets, req.Reps, req.WeightLb)
	if !ok {
		badRequest(w, msg)
		return
	}

	ctx := r.Context()
	userID := userFrom(ctx).ID

	session, err := s.q.GetSession(ctx, store.GetSessionParams{ID: sessionID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "session not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}
	// Same rule as adding a set, and for the same reason: a lift succeeds only
	// when every one of its sets was completed, so bolting unlogged sets onto a
	// closed session can retroactively turn a success into a failure and move
	// the next session's weight.
	//
	// It bites harder here than there. A queued write that sits overnight
	// replays into a session the twelve-hour rule has since closed, and what is
	// refused is not one appended set but a whole exercise — along with every
	// rep the lifter logged against it while offline.
	if session.IsOver {
		conflict(w, "session_over", "this session is finished")
		return
	}

	// User-scoped, so another lifter's custom movement can neither be attached
	// nor probed for existence by watching which ids 404.
	ex, err := s.q.GetExercise(ctx, store.GetExerciseParams{ID: req.ExerciseID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "exercise not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	if code, msg, ok := s.assistanceFitsDay(ctx, userID, session.ProgramDayID, ex.ID); !ok {
		conflict(w, code, msg)
		return
	}

	// The lift must not already be in THIS session, which is the guard the
	// program-day path has no need of. session_sets is UNIQUE on
	// (session_id, exercise_id, set_number), so materialising a second set
	// number 1 for a lift already here would fail the insert — and a client
	// should have filtered it out of the picker long before that.
	existing, err := s.q.ListSessionSets(ctx, store.ListSessionSetsParams{
		SessionID: sessionID, UserID: userID,
	})
	if err != nil {
		internalError(w)
		return
	}
	for _, set := range existing {
		if set.ExerciseID == ex.ID {
			conflict(w, "already_in_session", "that lift is already in this workout")
			return
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	if _, err := qtx.CreateAssistance(ctx, store.CreateAssistanceParams{
		UserID:       userID,
		ProgramDayID: session.ProgramDayID,
		ExerciseID:   ex.ID,
		Sets:         req.Sets,
		Reps:         req.Reps,
		WeightLb:     floatToNumeric(weight),
	}); err != nil {
		// The checks above raced another writer. Report it as the conflict it
		// is rather than as a 500.
		if isUniqueViolation(err) {
			conflict(w, "duplicate_assistance", "that exercise is already on this day")
			return
		}
		internalError(w)
		return
	}

	// The weight and reps the lifter typed, used verbatim. The carry-forward
	// and rep progression in prescribe() take over from the NEXT session — for
	// today, overriding a number somebody just entered would be a surprise, and
	// it is what lets a client draw these sets before the server answers.
	for n := int32(1); n <= req.Sets; n++ {
		if _, err := qtx.CreateSessionSet(ctx, store.CreateSessionSetParams{
			SessionID:  sessionID,
			ExerciseID: ex.ID,
			SetNumber:  n,
			TargetReps: req.Reps,
			WeightLb:   floatToNumeric(weight),
		}); err != nil {
			internalError(w)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	// Re-read rather than trust the inserts' RETURNING, exactly as addSessionSet
	// does: exerciseName, restSeconds and isAssistance are all derived at read
	// time and none of them are on the insert. ListSessionSets orders by set
	// number last, so the filtered result is in the order the response promises.
	created, err := s.q.ListSessionSets(ctx, store.ListSessionSetsParams{
		SessionID: sessionID, UserID: userID,
	})
	if err != nil {
		internalError(w)
		return
	}
	out := make([]sessionSetDTO, 0, req.Sets)
	for _, set := range created {
		if set.ExerciseID != ex.ID {
			continue
		}
		out = append(out, sessionSetToDTO(set))
	}
	writeJSON(w, http.StatusCreated, out)
}
