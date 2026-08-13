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

func (s *Server) listExercises(w http.ResponseWriter, r *http.Request) {
	rows, err := s.q.ListExercises(r.Context())
	if err != nil {
		internalError(w)
		return
	}
	out := make([]exerciseDTO, 0, len(rows))
	for _, e := range rows {
		out = append(out, exerciseDTO{ID: e.ID, Name: e.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getExerciseHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "exerciseId")
	if !ok {
		notFound(w, "exercise not found")
		return
	}
	rows, err := s.q.ListExerciseHistory(r.Context(), id)
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

	dayDTOs := make([]programDayDTO, 0, len(days))
	for _, d := range days {
		exercises := byDay[d.ID]
		if exercises == nil {
			exercises = []programDayExerciseDTO{}
		}
		dayDTOs = append(dayDTOs, programDayDTO{
			ID: d.ID, Name: d.Name, Position: d.Position, Weekday: d.Weekday, Exercises: exercises,
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
	programID, ok := idParam(r, "programId")
	if !ok {
		notFound(w, "program day not found")
		return
	}
	dayID, ok := idParam(r, "dayId")
	if !ok {
		notFound(w, "program day not found")
		return
	}
	ctx := r.Context()

	day, err := s.q.GetProgramDay(ctx, dayID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && day.ProgramID != programID) {
		notFound(w, "program day not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	exercises, err := s.prescribe(ctx, programID, day.ID)
	if err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, prescribedSessionDTO{
		ProgramID:      programID,
		ProgramDayID:   day.ID,
		ProgramDayName: day.Name,
		Exercises:      exercises,
	})
}

// prescribe computes the next-session prescription for a day: each prescribed
// exercise with a target weight from the progression engine, applied over the
// lift's history within the program. Shared by preview and session creation.
func (s *Server) prescribe(ctx context.Context, programID, dayID int32) ([]prescribedExerciseDTO, error) {
	pres, err := s.q.ListPrescriptionsByDay(ctx, dayID)
	if err != nil {
		return nil, err
	}

	out := make([]prescribedExerciseDTO, 0, len(pres))
	for _, p := range pres {
		hist, err := s.q.ListLiftHistory(ctx, store.ListLiftHistoryParams{
			ProgramID: programID, ExerciseID: p.ExerciseID,
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
	return out, nil
}
