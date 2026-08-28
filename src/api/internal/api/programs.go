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
			RestSeconds:      pr.RestSeconds,
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

	userID := userFrom(ctx).ID
	// Measured on every preview, not only when ?deload=true, because this is
	// where the client learns there is a question to ask at all. Applied only
	// when asked.
	lay, err := s.layoffFor(ctx, userID, r.URL.Query().Get("deload") == "true")
	if err != nil {
		internalError(w)
		return
	}

	exercises, err := s.prescribe(ctx, day.ProgramID, day.ID, userID, lay)
	if err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, prescribedSessionDTO{
		ProgramID:      day.ProgramID,
		ProgramDayID:   day.ID,
		ProgramDayName: day.Name,
		Exercises:      exercises,
		Layoff:         lay.dto(),
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
//
// lay is how long the lifter has been away and whether they asked to ease back
// in. When it is active every weight below is cut by the same fraction, main
// lifts and assistance alike — see layoff.go.
func (s *Server) prescribe(ctx context.Context, programID, dayID, userID int32, lay layoffState) ([]prescribedExerciseDTO, error) {
	pres, err := s.q.ListPrescriptionsByDay(ctx, dayID)
	if err != nil {
		return nil, err
	}

	// The lifts the program itself prescribes today. Used below to drop an
	// assistance entry that names one of them: addAssistance refuses to create
	// such a row, but this is what makes the invariant hold rather than merely
	// being checked. A row can predate that guard, and a seed migration adding a
	// lift to a day would create the same collision from the other side.
	//
	// Emitting the lift twice does not degrade gracefully — createSession would
	// insert two sets numbered 1 for it and trip session_sets' UNIQUE, turning
	// every attempt to start the workout into a 500. Skipping is the behaviour
	// that keeps the day usable; the program's own prescription wins, because it
	// is the one with a progression behind it.
	prescribed := make(map[int32]bool, len(pres))
	for _, p := range pres {
		prescribed[p.ExerciseID] = true
	}

	out := make([]prescribedExerciseDTO, 0, len(pres))
	for _, p := range pres {
		hist, err := s.q.ListLiftHistory(ctx, store.ListLiftHistoryParams{
			ExerciseID: p.ExerciseID, UserID: userID,
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
		if lay.active() {
			plan = progression.ApplyLayoff(plan, lay.weeks)
		}
		out = append(out, prescribedExerciseDTO{
			ExerciseID:   p.ExerciseID,
			ExerciseName: p.ExerciseName,
			Kind:         exerciseKindMain,
			Sets:         p.Sets,
			Reps:         p.Reps,
			WeightLb:     plan.WeightLb,
			RestSeconds:  p.RestSeconds,
			Progression: progressionInfoDTO{
				Status:               string(plan.Status),
				FailureCount:         plan.FailureCount,
				FailuresBeforeDeload: progression.FailuresBeforeDeload,
				PreviousWeightLb:     plan.PreviousLb,
				LayoffPct:            plan.LayoffPct,
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
		if prescribed[a.ExerciseID] {
			continue
		}
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

		// A layoff does reach assistance, which is the one thing that overrides
		// the paragraph above. It is not the engine arriving by the back door:
		// no progression is being computed here, the same flat fraction is
		// coming off every lift in the session. Three weeks out of the gym cost
		// the curl what they cost the squat, and a lifter who agreed to ease
		// back in did not mean "except the accessories".
		//
		// Only for a lift with history, matching ApplyLayoff's StatusStart
		// guard: a stored fallback weight is what to use the first time, not
		// something to detrain off.
		layoffPct := 0.0
		if lay.active() && previous > 0 {
			weight = progression.LayoffWeight(previous, lay.weeks)
			layoffPct = progression.LayoffPct(lay.weeks)
		}

		status := progressionFixed
		if layoffPct > 0 {
			status = string(progression.StatusLayoff)
		}
		out = append(out, prescribedExerciseDTO{
			ExerciseID:   a.ExerciseID,
			ExerciseName: a.ExerciseName,
			Kind:         exerciseKindAssistance,
			Sets:         a.Sets,
			Reps:         a.Reps,
			WeightLb:     weight,
			RestSeconds:  a.RestSeconds,
			Progression: progressionInfoDTO{
				Status:               status,
				FailuresBeforeDeload: progression.FailuresBeforeDeload,
				PreviousWeightLb:     previous,
				LayoffPct:            layoffPct,
			},
		})
	}
	return out, nil
}
