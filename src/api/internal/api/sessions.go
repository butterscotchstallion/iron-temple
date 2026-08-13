package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// pagination defaults for GET /sessions (mirrors openapi.yaml).
const (
	defaultLimit = 20
	maxLimit     = 100
)

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	limit := int32(defaultLimit)
	if v := query.Get("limit"); v != "" {
		// ParseInt with bitSize 32 rejects values that don't fit int32, so the
		// int32(n) conversion below can't overflow (also clears gosec G109/G115).
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 1 || n > maxLimit {
			badRequest(w, "limit must be between 1 and 100")
			return
		}
		limit = int32(n)
	}

	offset := int32(0)
	if v := query.Get("offset"); v != "" {
		// ParseInt with bitSize 32 bounds n to int32 range — an out-of-range offset
		// (e.g. 3000000000) is now a 400 instead of silently wrapping negative on the
		// int32(n) conversion. Previously offset only checked the lower bound.
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 0 {
			badRequest(w, "offset must be >= 0")
			return
		}
		offset = int32(n)
	}

	var programID *int64
	if v := query.Get("programId"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			badRequest(w, "programId must be a positive integer")
			return
		}
		programID = &n
	}

	rows, err := s.q.ListSessions(ctx, store.ListSessionsParams{
		ProgramID: programID, Off: offset, Lim: limit,
	})
	if err != nil {
		internalError(w)
		return
	}
	total, err := s.q.CountSessions(ctx, programID)
	if err != nil {
		internalError(w)
		return
	}

	// Per-exercise top weights for the sessions on this page, grouped by session.
	ids := make([]int32, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	weightsBySession := make(map[int32][]sessionExerciseWeightDTO, len(rows))
	if len(ids) > 0 {
		weights, err := s.q.ListSessionExerciseWeights(ctx, ids)
		if err != nil {
			internalError(w)
			return
		}
		for _, wt := range weights {
			weightsBySession[wt.SessionID] = append(weightsBySession[wt.SessionID], sessionExerciseWeightDTO{
				ExerciseName: wt.ExerciseName,
				Sets:         wt.SetCount,
				Reps:         wt.Reps,
				WeightLb:     numericToFloat(wt.WeightLb),
			})
		}
	}

	items := make([]sessionSummaryDTO, 0, len(rows))
	for _, row := range rows {
		exercises := weightsBySession[row.ID]
		if exercises == nil {
			exercises = []sessionExerciseWeightDTO{}
		}
		items = append(items, sessionSummaryDTO{
			ID:                row.ID,
			ProgramID:         row.ProgramID,
			ProgramName:       row.ProgramName,
			ProgramDayID:      row.ProgramDayID,
			ProgramDayName:    row.ProgramDayName,
			PerformedOn:       dateToString(row.PerformedOn),
			SetCount:          row.SetCount,
			CompletedSetCount: row.CompletedSetCount,
			IsOver:            row.IsOver,
			Exercises:         exercises,
		})
	}

	writeJSON(w, http.StatusOK, sessionListDTO{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

type createSessionRequest struct {
	ProgramDayID int32   `json:"programDayId"`
	PerformedOn  *string `json:"performedOn"`
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.ProgramDayID <= 0 {
		badRequest(w, "programDayId is required")
		return
	}

	performedOn := dateToday()
	if req.PerformedOn != nil {
		d, err := parseDate(*req.PerformedOn)
		if err != nil {
			badRequest(w, "performedOn must be a YYYY-MM-DD date")
			return
		}
		performedOn = d
	}

	day, err := s.q.GetProgramDay(ctx, req.ProgramDayID)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "program day not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	prescription, err := s.prescribe(ctx, day.ProgramID, day.ID)
	if err != nil {
		internalError(w)
		return
	}

	// Materialize the session and its sets atomically.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	session, err := qtx.CreateSession(ctx, store.CreateSessionParams{
		ProgramDayID: day.ID, PerformedOn: performedOn,
	})
	if err != nil {
		internalError(w)
		return
	}
	for _, pe := range prescription {
		weight := floatToNumeric(pe.WeightLb)
		for setNumber := int32(1); setNumber <= pe.Sets; setNumber++ {
			if _, err := qtx.CreateSessionSet(ctx, store.CreateSessionSetParams{
				SessionID:  session.ID,
				ExerciseID: pe.ExerciseID,
				SetNumber:  setNumber,
				TargetReps: pe.Reps,
				WeightLb:   weight,
			}); err != nil {
				internalError(w)
				return
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	full, err := s.buildSession(ctx, session.ID)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusCreated, full)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "sessionId")
	if !ok {
		notFound(w, "session not found")
		return
	}
	full, err := s.buildSession(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "session not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, full)
}

type updateSessionRequest struct {
	PerformedOn *string `json:"performedOn"`
	Notes       *string `json:"notes"`
}

func (s *Server) updateSession(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "sessionId")
	if !ok {
		notFound(w, "session not found")
		return
	}
	ctx := r.Context()

	var req updateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	params := store.UpdateSessionParams{ID: id, Notes: req.Notes}
	if req.PerformedOn != nil {
		d, err := parseDate(*req.PerformedOn)
		if err != nil {
			badRequest(w, "performedOn must be a YYYY-MM-DD date")
			return
		}
		params.PerformedOn = d
	}

	if _, err := s.q.UpdateSession(ctx, params); errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "session not found")
		return
	} else if err != nil {
		internalError(w)
		return
	}

	full, err := s.buildSession(ctx, id)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, full)
}

// finishSession marks a session as ended by hand. It is idempotent — the store
// keeps the first timestamp — so a double-tap on Finish is harmless.
func (s *Server) finishSession(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "sessionId")
	if !ok {
		notFound(w, "session not found")
		return
	}
	ctx := r.Context()

	if _, err := s.q.FinishSession(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "session not found")
		return
	} else if err != nil {
		internalError(w)
		return
	}

	full, err := s.buildSession(ctx, id)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, full)
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "sessionId")
	if !ok {
		notFound(w, "session not found")
		return
	}
	n, err := s.q.DeleteSession(r.Context(), id)
	if err != nil {
		internalError(w)
		return
	}
	if n == 0 {
		notFound(w, "session not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updateSessionSet(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := idParam(r, "sessionId")
	if !ok {
		notFound(w, "set not found")
		return
	}
	setID, ok := idParam(r, "setId")
	if !ok {
		notFound(w, "set not found")
		return
	}
	ctx := r.Context()

	current, err := s.q.GetSessionSet(ctx, setID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && current.SessionID != sessionID) {
		notFound(w, "set not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	// Decode into raw fields so we can tell "absent" from "explicit null" — the
	// spec lets any subset be sent, and actualReps: null clears a prior entry.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	params := store.UpdateSessionSetParams{
		ID:         setID,
		ActualReps: current.ActualReps,
		WeightLb:   current.WeightLb,
		Completed:  current.Completed,
	}

	if v, ok := raw["actualReps"]; ok {
		if string(v) == "null" {
			params.ActualReps = nil
		} else {
			var reps int32
			if err := json.Unmarshal(v, &reps); err != nil || reps < 0 {
				badRequest(w, "actualReps must be a non-negative integer or null")
				return
			}
			params.ActualReps = &reps
		}
	}
	if v, ok := raw["weightLb"]; ok {
		var weight float64
		if err := json.Unmarshal(v, &weight); err != nil || weight < 0 {
			badRequest(w, "weightLb must be a non-negative number")
			return
		}
		params.WeightLb = floatToNumeric(weight)
	}
	if v, ok := raw["completed"]; ok {
		var completed bool
		if err := json.Unmarshal(v, &completed); err != nil {
			badRequest(w, "completed must be a boolean")
			return
		}
		params.Completed = completed
	}

	updated, err := s.q.UpdateSessionSet(ctx, params)
	if err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, sessionSetDTO{
		ID:           updated.ID,
		ExerciseID:   updated.ExerciseID,
		ExerciseName: current.ExerciseName,
		SetNumber:    updated.SetNumber,
		TargetReps:   updated.TargetReps,
		ActualReps:   updated.ActualReps,
		WeightLb:     numericToFloat(updated.WeightLb),
		Completed:    updated.Completed,
		RestSeconds:  restSecondsDefault,
	})
}

// buildSession assembles the full session response (metadata + ordered sets).
// Returns pgx.ErrNoRows when the session does not exist.
func (s *Server) buildSession(ctx context.Context, id int32) (sessionDTO, error) {
	g, err := s.q.GetSession(ctx, id)
	if err != nil {
		return sessionDTO{}, err
	}
	sets, err := s.q.ListSessionSets(ctx, id)
	if err != nil {
		return sessionDTO{}, err
	}

	setDTOs := make([]sessionSetDTO, 0, len(sets))
	for _, set := range sets {
		setDTOs = append(setDTOs, sessionSetDTO{
			ID:           set.ID,
			ExerciseID:   set.ExerciseID,
			ExerciseName: set.ExerciseName,
			SetNumber:    set.SetNumber,
			TargetReps:   set.TargetReps,
			ActualReps:   set.ActualReps,
			WeightLb:     numericToFloat(set.WeightLb),
			Completed:    set.Completed,
			RestSeconds:  restSecondsDefault,
		})
	}

	return sessionDTO{
		ID:             g.ID,
		ProgramID:      g.ProgramID,
		ProgramName:    g.ProgramName,
		ProgramDayID:   g.ProgramDayID,
		ProgramDayName: g.ProgramDayName,
		PerformedOn:    dateToString(g.PerformedOn),
		Notes:          g.Notes,
		CreatedAt:      timestamptzToString(g.CreatedAt),
		FinishedAt:     optionalTimestamptz(g.FinishedAt),
		IsOver:         g.IsOver,
		Sets:           setDTOs,
	}, nil
}
