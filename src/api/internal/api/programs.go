package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/progression"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

func (s *Server) getHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthDTO{Status: "ok", Version: s.version, Environment: s.environment})
}

func (s *Server) listPrograms(w http.ResponseWriter, r *http.Request) {
	rows, err := s.q.ListPrograms(r.Context())
	if err != nil {
		internalError(w)
		return
	}
	out := make([]programSummaryDTO, 0, len(rows))
	for _, p := range rows {
		out = append(out, programSummaryDTO{
			ID: p.ID, Name: p.Name, Description: p.Description, ProgressionKind: p.ProgressionKind,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getProgram(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "programId")
	if !ok {
		notFound(w, "program not found")
		return
	}
	ctx := r.Context()

	p, err := s.q.GetProgram(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "program not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	days, err := s.q.ListProgramDays(ctx, id)
	if err != nil {
		internalError(w)
		return
	}
	pres, err := s.q.ListPrescriptionsByProgram(ctx, id)
	if err != nil {
		internalError(w)
		return
	}
	// The caller's own assistance across every day of the program, fetched
	// alongside the shared prescription rather than per day: the response is one
	// object, so it should cost one round trip.
	assist, err := s.q.ListAssistanceByProgram(ctx, store.ListAssistanceByProgramParams{
		ProgramID: id, UserID: userFrom(ctx).ID,
	})
	if err != nil {
		internalError(w)
		return
	}

	// Group prescriptions under their day (query is ordered by day then position).
	byDay := make(map[int32][]programDayExerciseDTO, len(days))
	for _, pr := range pres {
		byDay[pr.ProgramDayID] = append(byDay[pr.ProgramDayID], programDayExerciseDTO{
			ID:               pr.ID,
			ExerciseID:       pr.ExerciseID,
			ExerciseName:     pr.ExerciseName,
			Position:         pr.Position,
			Sets:             pr.Sets,
			Reps:             pr.Reps,
			StartingWeightLb: numericToFloat(pr.StartingWeightLb),
			RestSeconds:      restSecondsDefault,
		})
	}

	assistByDay := make(map[int32][]programDayAssistanceDTO, len(days))
	for _, a := range assist {
		assistByDay[a.ProgramDayID] = append(assistByDay[a.ProgramDayID], programDayAssistanceDTO{
			ID:           a.ID,
			ExerciseID:   a.ExerciseID,
			ExerciseName: a.ExerciseName,
			Position:     a.Position,
			Sets:         a.Sets,
			Reps:         a.Reps,
			WeightLb:     numericToFloat(a.WeightLb),
		})
	}

	dayDTOs := make([]programDayDTO, 0, len(days))
	for _, d := range days {
		exercises := byDay[d.ID]
		if exercises == nil {
			exercises = []programDayExerciseDTO{}
		}
		assistance := assistByDay[d.ID]
		if assistance == nil {
			assistance = []programDayAssistanceDTO{}
		}
		dayDTOs = append(dayDTOs, programDayDTO{
			ID: d.ID, Name: d.Name, Position: d.Position, Weekday: d.Weekday,
			Exercises: exercises, Assistance: assistance,
		})
	}

	writeJSON(w, http.StatusOK, programDTO{
		programSummaryDTO: programSummaryDTO{
			ID: p.ID, Name: p.Name, Description: p.Description, ProgressionKind: p.ProgressionKind,
		},
		Days: dayDTOs,
	})
}

type updateProgramDayRequest struct {
	Weekday *int32 `json:"weekday"`
}

func (s *Server) updateProgramDayWeekday(w http.ResponseWriter, r *http.Request) {
	dayID, ok := idParam(r, "dayId")
	if !ok {
		notFound(w, "program day not found")
		return
	}

	var req updateProgramDayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Weekday != nil && (*req.Weekday < 0 || *req.Weekday > 6) {
		badRequest(w, "weekday must be between 0 and 6")
		return
	}

	if _, err := s.q.UpdateProgramDayWeekday(r.Context(), store.UpdateProgramDayWeekdayParams{
		ID: dayID, Weekday: req.Weekday,
	}); errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "program day not found")
		return
	} else if err != nil {
		internalError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) previewNextSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day, ok := s.programDay(w, r)
	if !ok {
		return
	}

	exercises, err := s.prescribe(ctx, day.ProgramID, day.ID, userFrom(ctx).ID)
	if err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, prescribedSessionDTO{
		ProgramID:      day.ProgramID,
		ProgramDayID:   day.ID,
		ProgramDayName: day.Name,
		Exercises:      exercises,
	})
}

// prescribe computes the next-session prescription for a day: the program's own
// exercises with a target weight from the progression engine, then the lifter's
// assistance work. Shared by preview and session creation, so the two can never
// disagree about what a session contains.
//
// userID scopes the history. The prescription itself is shared — everyone on
// StrongLifts squats 5x5 — but the weight on the bar comes from what *this*
// lifter has done, so an unscoped history here would put someone else's numbers
// in front of them. Assistance goes further and is per-user all the way down:
// the exercises are theirs, not the program's.
//
// Order matters. Main lifts come first because the session materializes sets in
// this order and ListSessionSets reads them back in it — assistance is what you
// do after the barbell work, not instead of it.
func (s *Server) prescribe(ctx context.Context, programID, dayID, userID int32) ([]prescribedExerciseDTO, error) {
	pres, err := s.q.ListPrescriptionsByDay(ctx, dayID)
	if err != nil {
		return nil, err
	}

	out := make([]prescribedExerciseDTO, 0, len(pres))
	for _, p := range pres {
		hist, err := s.q.ListLiftHistory(ctx, store.ListLiftHistoryParams{
			ProgramID: programID, ExerciseID: p.ExerciseID, UserID: userID,
		})
		if err != nil {
			return nil, err
		}
		history := make([]progression.SessionResult, 0, len(hist))
		for _, h := range hist {
			history = append(history, progression.SessionResult{
				WeightLb: numericToFloat(h.WeightLb), Success: h.Success,
			})
		}
		plan := progression.NextPlan(
			numericToFloat(p.StartingWeightLb),
			progression.IncrementFor(p.ExerciseName),
			history,
		)
		out = append(out, prescribedExerciseDTO{
			ExerciseID:   p.ExerciseID,
			ExerciseName: p.ExerciseName,
			Kind:         exerciseKindMain,
			Sets:         p.Sets,
			Reps:         p.Reps,
			WeightLb:     plan.WeightLb,
			RestSeconds:  restSecondsDefault,
			Progression: progressionInfoDTO{
				Status:               string(plan.Status),
				FailureCount:         plan.FailureCount,
				FailuresBeforeDeload: progression.FailuresBeforeDeload,
				PreviousWeightLb:     plan.PreviousLb,
			},
		})
	}

	assist, err := s.q.ListAssistanceByDay(ctx, store.ListAssistanceByDayParams{
		ProgramDayID: dayID, UserID: userID,
	})
	if err != nil {
		return nil, err
	}
	for _, a := range assist {
		// No engine runs on assistance. A curl is not a squat: it does not
		// advance five pounds a session, and stalling on one is not a signal
		// worth deloading over. So the rule is the one lifters already follow —
		// do what you did last time, and change it when you feel like it. The
		// weight carries forward from the last session that logged the lift,
		// which means editing it mid-workout is how you change it, and the
		// stored weight is only the fallback for a lift never performed.
		//
		// ListExerciseHistory is reused rather than a new query written for it:
		// it already returns one point per session, user-scoped, oldest first,
		// with the top weight worked. The last point is what we want. It is not
		// scoped to the program, deliberately — dips are dips whichever day they
		// were done on.
		hist, err := s.q.ListExerciseHistory(ctx, store.ListExerciseHistoryParams{
			ExerciseID: a.ExerciseID, UserID: userID,
		})
		if err != nil {
			return nil, err
		}
		weight := numericToFloat(a.WeightLb)
		previous := 0.0
		if n := len(hist); n > 0 {
			previous = numericToFloat(hist[n-1].WeightLb)
			weight = previous
		}
		out = append(out, prescribedExerciseDTO{
			ExerciseID:   a.ExerciseID,
			ExerciseName: a.ExerciseName,
			Kind:         exerciseKindAssistance,
			Sets:         a.Sets,
			Reps:         a.Reps,
			WeightLb:     weight,
			RestSeconds:  restSecondsDefault,
			Progression: progressionInfoDTO{
				Status:               progressionFixed,
				FailuresBeforeDeload: progression.FailuresBeforeDeload,
				PreviousWeightLb:     previous,
			},
		})
	}
	return out, nil
}
