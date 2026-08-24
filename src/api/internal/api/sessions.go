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

	userID := userFrom(ctx).ID
	rows, err := s.q.ListSessions(ctx, store.ListSessionsParams{
		UserID: userID, ProgramID: programID, Off: offset, Lim: limit,
	})
	if err != nil {
		internalError(w)
		return
	}
	totals, err := s.q.SessionTotals(ctx, store.SessionTotalsParams{
		UserID: userID, ProgramID: programID,
	})
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
		weights, err := s.q.ListSessionExerciseWeights(ctx, store.ListSessionExerciseWeightsParams{
			SessionIds: ids, UserID: userID,
		})
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
			VolumeLb:          numericToFloat(row.VolumeLb),
			IsOver:            row.IsOver,
			Exercises:         exercises,
		})
	}

	writeJSON(w, http.StatusOK, sessionListDTO{
		Items: items, Total: totals.Total, TotalVolumeLb: numericToFloat(totals.VolumeLb),
		Limit: limit, Offset: offset,
	})
}

type createSessionRequest struct {
	ProgramDayID int32   `json:"programDayId"`
	PerformedOn  *string `json:"performedOn"`
	// Deload asks for the layoff cut to be baked into this session's weights —
	// the lifter's yes to the prompt the preview raised. Absent reads as no,
	// which is what makes a client that has never heard of the feature behave
	// exactly as it did before.
	//
	// Not a weight or a percentage: how deep the cut goes is the server's to
	// decide from how long they have actually been away (layoff.go), so this
	// cannot be used to prescribe an arbitrary number.
	Deload bool `json:"deload"`
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

	userID := userFrom(ctx).ID
	// Measured now rather than trusted from the client: the preview that raised
	// the prompt may be minutes or a reload old, and the length of the layoff
	// decides the size of the cut.
	lay, err := s.layoffFor(ctx, userID, req.Deload)
	if err != nil {
		internalError(w)
		return
	}
	prescription, err := s.prescribe(ctx, day.ProgramID, day.ID, userID, lay)
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
		ProgramDayID: day.ID, PerformedOn: performedOn, UserID: userID,
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

	full, err := s.buildSession(ctx, session.ID, userID)
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
	ctx := r.Context()
	full, err := s.buildSession(ctx, id, userFrom(ctx).ID)
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

// maxBodyweightLb bounds a weigh-in. It is a typo catch rather than a claim
// about people: the heaviest human on record was under 1,500 lb, and the column
// is NUMERIC(6,2), so anything this side of the limit stores fine. Its job is to
// turn a fat-fingered 18450 into a 400 rather than a row nobody meant to write.
const maxBodyweightLb = 2000

func (s *Server) updateSession(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "sessionId")
	if !ok {
		notFound(w, "session not found")
		return
	}
	ctx := r.Context()

	// Decoded as raw fields rather than a struct, for the same reason
	// updateSessionSet does it: bodyweightLb absent must differ from
	// bodyweightLb: null. The first leaves a weigh-in alone, the second erases
	// it, and a *float64 renders both as nil.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	userID := userFrom(ctx).ID
	params := store.UpdateSessionParams{ID: id, UserID: userID}

	// performedOn and notes keep their original semantics, where a null is the
	// same as omitting the field — the store COALESCEs them, so neither can be
	// cleared and null asks for nothing. Only bodyweightLb reads a null as an
	// instruction, and only it needs the distinction.
	if v, ok := raw["performedOn"]; ok && string(v) != "null" {
		var performedOn string
		if err := json.Unmarshal(v, &performedOn); err != nil {
			badRequest(w, "performedOn must be a YYYY-MM-DD date")
			return
		}
		d, err := parseDate(performedOn)
		if err != nil {
			badRequest(w, "performedOn must be a YYYY-MM-DD date")
			return
		}
		params.PerformedOn = d
	}
	if v, ok := raw["notes"]; ok && string(v) != "null" {
		var notes string
		if err := json.Unmarshal(v, &notes); err != nil {
			badRequest(w, "notes must be a string")
			return
		}
		params.Notes = &notes
	}
	if v, ok := raw["bodyweightLb"]; ok {
		params.SetBodyweight = true
		if string(v) != "null" {
			var weight float64
			if err := json.Unmarshal(v, &weight); err != nil || weight <= 0 || weight > maxBodyweightLb {
				badRequest(w, "bodyweightLb must be a positive number up to 2000, or null")
				return
			}
			params.BodyweightLb = floatToNumeric(weight)
		}
	}

	if _, err := s.q.UpdateSession(ctx, params); errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "session not found")
		return
	} else if err != nil {
		internalError(w)
		return
	}

	full, err := s.buildSession(ctx, id, userID)
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

	userID := userFrom(ctx).ID
	if _, err := s.q.FinishSession(ctx, store.FinishSessionParams{
		ID: id, UserID: userID,
	}); errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "session not found")
		return
	} else if err != nil {
		internalError(w)
		return
	}

	full, err := s.buildSession(ctx, id, userID)
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
	n, err := s.q.DeleteSession(r.Context(), store.DeleteSessionParams{
		ID: id, UserID: userFrom(r.Context()).ID,
	})
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

	userID := userFrom(ctx).ID
	current, err := s.q.GetSessionSet(ctx, store.GetSessionSetParams{ID: setID, UserID: userID})
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
		UserID:     userID,
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
		Kind:         setKind(current.IsAssistance),
		SetNumber:    updated.SetNumber,
		TargetReps:   updated.TargetReps,
		ActualReps:   updated.ActualReps,
		WeightLb:     numericToFloat(updated.WeightLb),
		Completed:    updated.Completed,
		RestSeconds:  current.RestSeconds,
	})
}

// buildSession assembles the full session response (metadata + ordered sets).
// Returns pgx.ErrNoRows when the session does not exist *or* belongs to someone
// else — the caller turns that into a 404 either way, so a probe cannot tell a
// missing id from another user's.
func (s *Server) buildSession(ctx context.Context, id, userID int32) (sessionDTO, error) {
	g, err := s.q.GetSession(ctx, store.GetSessionParams{ID: id, UserID: userID})
	if err != nil {
		return sessionDTO{}, err
	}
	sets, err := s.q.ListSessionSets(ctx, store.ListSessionSetsParams{
		SessionID: id, UserID: userID,
	})
	if err != nil {
		return sessionDTO{}, err
	}

	// The weigh-in to pre-fill this session's box with, if the lifter has one on
	// another session. ErrNoRows means they don't, and must be swallowed here:
	// every caller reads an error out of buildSession as "no such session" and
	// answers 404, so letting it escape would make a first-ever session vanish.
	var lastWeighIn *weighInDTO
	last, err := s.q.LastWeighIn(ctx, store.LastWeighInParams{
		UserID: userID, ExcludeSessionID: id,
	})
	switch {
	case err == nil:
		lastWeighIn = &weighInDTO{
			WeightLb:    numericToFloat(last.BodyweightLb),
			PerformedOn: dateToString(last.PerformedOn),
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return sessionDTO{}, err
	}

	setDTOs := make([]sessionSetDTO, 0, len(sets))
	for _, set := range sets {
		setDTOs = append(setDTOs, sessionSetDTO{
			ID:           set.ID,
			ExerciseID:   set.ExerciseID,
			ExerciseName: set.ExerciseName,
			Kind:         setKind(set.IsAssistance),
			SetNumber:    set.SetNumber,
			TargetReps:   set.TargetReps,
			ActualReps:   set.ActualReps,
			WeightLb:     numericToFloat(set.WeightLb),
			Completed:    set.Completed,
			RestSeconds:  set.RestSeconds,
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
		BodyweightLb:   optionalNumeric(g.BodyweightLb),
		LastWeighIn:    lastWeighIn,
		Sets:           setDTOs,
	}, nil
}
