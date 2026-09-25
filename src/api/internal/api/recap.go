package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"gitea.homelab/gitadmin/iron-temple/api/internal/racked"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// getSessionRecap serves one session in review.
//
// Served whatever state the session is in. Refusing an unfinished one would be
// the tidier rule and the wrong one: finishing is a write, a write made in a
// basement is queued rather than sent, and the lifter whose Finish has not
// landed yet is exactly the lifter standing in front of the screen. An
// unfinished session simply has no duration and no pace.
func (s *Server) getSessionRecap(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "sessionId")
	if !ok {
		notFound(w, "session not found")
		return
	}
	ctx := r.Context()
	rec, earned, err := s.buildSessionRecap(ctx, id, userFrom(ctx).ID)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "session not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, sessionRecapToDTO(rec, earned))
}

// earnedNext is what the session bought: the next performance of the same
// program day, as the engine now prescribes it.
//
// Kept out of internal/racked, which reduces logged work to statistics and has
// no business knowing what a prescription is. This is a forecast rather than a
// finding, so it is assembled here and attached at the DTO boundary.
type earnedNext struct {
	ProgramDayID   int32
	ProgramDayName string
	Exercises      []prescribedExerciseDTO
}

// buildSessionRecap gathers a session's rows and reduces them to a recap.
//
// Six reads, none of them clever, in the shape buildRacked already established:
// the session, its sets, the previous performance of the same program day, that
// day's other lengths, the baseline that turns a maximum into a record, and the
// recent sessions a streak is counted over. Everything that looks like a
// statistic happens in internal/racked, over plain Go values.
//
// GetSession is what enforces ownership: it is scoped by user_id, so another
// lifter's session id returns ErrNoRows and the handler turns that into a 404.
// Every query below is scoped the same way, but this is the one that decides.
func (s *Server) buildSessionRecap(
	ctx context.Context, sessionID, userID int32,
) (racked.SessionRecap, *earnedNext, error) {
	head, err := s.q.GetSession(ctx, store.GetSessionParams{ID: sessionID, UserID: userID})
	if err != nil {
		return racked.SessionRecap{}, nil, err
	}
	meta := racked.SessionMeta{
		SessionID:      head.ID,
		ProgramID:      head.ProgramID,
		ProgramName:    head.ProgramName,
		ProgramDayID:   head.ProgramDayID,
		ProgramDayName: head.ProgramDayName,
		PerformedOn:    head.PerformedOn.Time,
		StartedAt:      head.CreatedAt.Time,
		FinishedAt:     finishedAt(head.FinishedAt),
		IsOver:         head.IsOver,
		BodyweightLb:   optionalNumeric(head.BodyweightLb),
	}

	sets, prescribed, err := s.recapSessionSets(ctx, meta, userID)
	if err != nil {
		return racked.SessionRecap{}, nil, err
	}

	// One read of this program day's history, used for two things: the newest
	// row is the session every "vs. last time" figure is measured against, and
	// the lengths of all of them are what pace is ranked within.
	history, err := s.q.RecapPreviousDaySessions(ctx, store.RecapPreviousDaySessionsParams{
		UserID:       userID,
		ProgramDayID: meta.ProgramDayID,
		PerformedOn:  pgtype.Date{Time: meta.PerformedOn, Valid: true},
		SessionID:    meta.SessionID,
	})
	if err != nil {
		return racked.SessionRecap{}, nil, err
	}
	prevMeta, prevSets, err := s.recapPreviousDay(ctx, meta, userID, history)
	if err != nil {
		return racked.SessionRecap{}, nil, err
	}
	durations := recapDayDurations(history)

	baseline, err := s.recapBaseline(ctx, meta, userID)
	if err != nil {
		return racked.SessionRecap{}, nil, err
	}
	outcomes, err := s.recapOutcomes(ctx, meta, userID)
	if err != nil {
		return racked.SessionRecap{}, nil, err
	}
	earned, err := s.recapEarned(ctx, meta, userID)
	if err != nil {
		return racked.SessionRecap{}, nil, err
	}

	return racked.BuildSession(racked.SessionInput{
		Meta:         meta,
		Sets:         sets,
		Prescribed:   prescribed,
		PreviousMeta: prevMeta,
		PreviousSets: prevSets,
		DayDurations: durations,
		Baseline:     baseline,
		Outcomes:     outcomes,
	}), earned, nil
}

// recapEarned computes what this session bought — the next performance of the
// same program day, at the weights the engine now prescribes.
//
// Nil unless this is the lifter's most recent session of the day. The
// prescription is derived from history as it stands now, so on the newest
// session it answers exactly the question the lifter is asking, and on an older
// one it would quietly describe a workout that has since been superseded while
// appearing to be a fact about the session on screen. A recap is a record of a
// moment; this is the one field in it that could drift, so it is withheld
// rather than qualified.
//
// The layoff is measured but not applied (apply=false), matching what the
// preview endpoint does by default: a lifter reading this has just trained, so
// there is no layoff to speak of, and cutting the weights on the strength of
// one would answer a question nobody asked.
func (s *Server) recapEarned(
	ctx context.Context, meta racked.SessionMeta, userID int32,
) (*earnedNext, error) {
	later, err := s.q.RecapHasLaterDaySession(ctx, store.RecapHasLaterDaySessionParams{
		UserID:       userID,
		ProgramDayID: meta.ProgramDayID,
		PerformedOn:  pgtype.Date{Time: meta.PerformedOn, Valid: true},
		SessionID:    meta.SessionID,
	})
	if err != nil {
		return nil, err
	}
	if later {
		return nil, nil
	}

	lay, err := s.layoffFor(ctx, userID, false)
	if err != nil {
		return nil, err
	}
	exercises, err := s.prescribe(ctx, meta.ProgramID, meta.ProgramDayID, userID, lay)
	if err != nil {
		return nil, err
	}
	return &earnedNext{
		ProgramDayID:   meta.ProgramDayID,
		ProgramDayName: meta.ProgramDayName,
		Exercises:      exercises,
	}, nil
}

// recapSessionSets reads the session once and returns both views of it: the
// logged sets that statistics are computed from, and every prescribed row,
// which is what gives "10 of 25 reps" a denominator.
//
// One query, two slices, rather than two queries — the unlogged rows are the
// only difference between them, and reading the table twice to split on a
// column we already have would be a round trip spent on arithmetic.
func (s *Server) recapSessionSets(
	ctx context.Context, meta racked.SessionMeta, userID int32,
) ([]racked.Set, []racked.Prescribed, error) {
	rows, err := s.q.RecapSessionSets(ctx, store.RecapSessionSetsParams{
		SessionID: meta.SessionID, UserID: userID,
	})
	if err != nil {
		return nil, nil, err
	}

	sets := make([]racked.Set, 0, len(rows))
	prescribed := make([]racked.Prescribed, 0, len(rows))
	for _, row := range rows {
		reps := 0
		if row.ActualReps != nil {
			reps = int(*row.ActualReps)
		}
		prescribed = append(prescribed, racked.Prescribed{
			ExerciseID:   row.ExerciseID,
			ExerciseName: row.ExerciseName,
			TargetReps:   int(row.TargetReps),
			Reps:         reps,
			Completed:    row.Completed,
			IsAssistance: row.IsAssistance,
			IsBonus:      row.IsBonus,
		})
		if reps <= 0 {
			continue
		}
		sets = append(sets, racked.Set{
			SessionID:      meta.SessionID,
			PerformedOn:    meta.PerformedOn,
			StartedAt:      meta.StartedAt,
			FinishedAt:     meta.FinishedAt,
			ProgramDayName: meta.ProgramDayName,
			ExerciseID:     row.ExerciseID,
			ExerciseName:   row.ExerciseName,
			MuscleGroup:    row.MuscleGroup,
			Equipment:      row.Equipment,
			Reps:           reps,
			WeightLb:       numericToFloat(row.WeightLb),
			Completed:      row.Completed,
			IsAssistance:   row.IsAssistance,
		})
	}
	return sets, prescribed, nil
}

// recapPreviousDay reads the sets of the last time this program day was
// performed. Both returns are nil when there was no last time.
//
// history is the same list RecapPreviousDaySets narrows itself to, newest
// first, so history[0] is the session those sets belong to. Passing it in
// rather than re-reading it is what keeps the two from having to be trusted to
// agree — they apply the same cut, but only one of them runs.
func (s *Server) recapPreviousDay(
	ctx context.Context, meta racked.SessionMeta, userID int32,
	history []store.RecapPreviousDaySessionsRow,
) (*racked.SessionMeta, []racked.Set, error) {
	if len(history) == 0 {
		return nil, nil, nil
	}
	rows, err := s.q.RecapPreviousDaySets(ctx, store.RecapPreviousDaySetsParams{
		UserID:       userID,
		ProgramDayID: meta.ProgramDayID,
		PerformedOn:  pgtype.Date{Time: meta.PerformedOn, Valid: true},
		SessionID:    meta.SessionID,
	})
	if err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	prev := history[0]

	prevMeta := racked.SessionMeta{
		SessionID:      prev.ID,
		ProgramID:      meta.ProgramID,
		ProgramDayID:   meta.ProgramDayID,
		ProgramDayName: meta.ProgramDayName,
		PerformedOn:    prev.PerformedOn.Time,
		StartedAt:      prev.CreatedAt.Time,
		FinishedAt:     finishedAt(prev.FinishedAt),
	}

	sets := make([]racked.Set, 0, len(rows))
	for _, row := range rows {
		reps := 0
		if row.ActualReps != nil {
			reps = int(*row.ActualReps)
		}
		sets = append(sets, racked.Set{
			SessionID:    prev.ID,
			PerformedOn:  prevMeta.PerformedOn,
			ExerciseID:   row.ExerciseID,
			ExerciseName: row.ExerciseName,
			Reps:         reps,
			WeightLb:     numericToFloat(row.WeightLb),
		})
	}
	return &prevMeta, sets, nil
}

// recapDayDurations reduces this program day's history to the lengths pace is
// ranked within.
//
// The 12-hour cap and the "never finished" rule are applied by calling the same
// helper internal/racked uses for its own durations, rather than being restated
// in SQL where they could drift from it. A session that yields no duration is
// dropped rather than passed along as a zero, so the median is taken over
// lengths that are all real.
func recapDayDurations(history []store.RecapPreviousDaySessionsRow) []time.Duration {
	out := make([]time.Duration, 0, len(history))
	for _, row := range history {
		d := racked.SessionDuration(row.CreatedAt.Time, finishedAt(row.FinishedAt))
		if d > 0 {
			out = append(out, d)
		}
	}
	return out
}

// recapBaseline is the lifter's history before this session — what makes a set
// inside it a record rather than merely the heaviest thing in one workout.
func (s *Server) recapBaseline(
	ctx context.Context, meta racked.SessionMeta, userID int32,
) (racked.Baseline, error) {
	on := pgtype.Date{Time: meta.PerformedOn, Valid: true}

	volume, err := s.q.RecapVolumeBefore(ctx, store.RecapVolumeBeforeParams{
		UserID: userID, PerformedOn: on, SessionID: meta.SessionID,
	})
	if err != nil {
		return racked.Baseline{}, err
	}
	rows, err := s.q.RecapExerciseBaseline(ctx, store.RecapExerciseBaselineParams{
		UserID: userID, PerformedOn: on, SessionID: meta.SessionID,
	})
	if err != nil {
		return racked.Baseline{}, err
	}

	return baselineFrom(volume, rows, func(
		r store.RecapExerciseBaselineRow,
	) (int32, pgtype.Numeric, pgtype.Numeric) {
		return r.ExerciseID, r.BestWeightLb, r.BestE1rmLb
	}), nil
}

// recapOutcomes reads the recent sessions both streaks are counted over.
func (s *Server) recapOutcomes(
	ctx context.Context, meta racked.SessionMeta, userID int32,
) ([]racked.SessionOutcome, error) {
	rows, err := s.q.RecapSessionOutcomes(ctx, store.RecapSessionOutcomesParams{
		UserID:      userID,
		PerformedOn: pgtype.Date{Time: meta.PerformedOn, Valid: true},
		SessionID:   meta.SessionID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]racked.SessionOutcome, 0, len(rows))
	for _, row := range rows {
		out = append(out, racked.SessionOutcome{
			PerformedOn:       row.PerformedOn.Time,
			SetCount:          int(row.SetCount),
			CompletedSetCount: int(row.CompletedSetCount),
		})
	}
	return out, nil
}

func sessionRecapToDTO(rec racked.SessionRecap, earned *earnedNext) sessionRecapDTO {
	out := sessionRecapDTO{
		Session: sessionRecapHeaderDTO{
			SessionID:      rec.Session.SessionID,
			ProgramID:      rec.Session.ProgramID,
			ProgramName:    rec.Session.ProgramName,
			ProgramDayID:   rec.Session.ProgramDayID,
			ProgramDayName: rec.Session.ProgramDayName,
			PerformedOn:    rec.Session.PerformedOn.Format(dateLayout),
			StartedAt:      rec.Session.StartedAt.UTC().Format(time.RFC3339),
			IsOver:         rec.Session.IsOver,
		},
		Volume: sessionRecapVolumeDTO{
			TotalLb:    rec.Volume.TotalLb,
			PreviousLb: rec.Volume.PreviousLb,
			DeltaPct:   rec.Volume.DeltaPct,
			Comparison: rackedComparisonDTO{
				Count:  rec.Volume.Comparison.Count,
				Label:  rec.Volume.Comparison.Label,
				UnitLb: rec.Volume.Comparison.UnitLb,
			},
			SetsLogged:     rec.Volume.SetsLogged,
			SetsPrescribed: rec.Volume.SetsPrescribed,
			RepsLogged:     rec.Volume.RepsLogged,
			RepsTargeted:   rec.Volume.RepsTargeted,
			SetsBonus:      rec.Volume.SetsBonus,
		},
		Progress: sessionRecapProgressDTO{
			PreviousSessionID: rec.Progress.PreviousSessionID,
			WeightDeltaPct:    rec.Progress.WeightDeltaPct,
			LiftsCompared:     rec.Progress.LiftsCompared,
			LiftsNew:          rec.Progress.LiftsNew,
		},
		Muscles: make([]rackedMuscleSliceDTO, 0, len(rec.Muscles)),
		Split: rackedSplitDTO{
			Main:       rackedWorkToDTO(rec.Split.Main),
			Assistance: rackedWorkToDTO(rec.Split.Assistance),
		},
		BodyweightLb: rec.Session.BodyweightLb,
		Lifts:        make([]sessionRecapLiftDTO, 0, len(rec.Lifts)),
		PRs:          make([]rackedPRDTO, 0, len(rec.PRs)),
		FirstTimes:   make([]rackedFirstTimeDTO, 0, len(rec.FirstTimes)),
		Milestones:   make([]rackedMilestoneDTO, 0, len(rec.Milestones)),
		Streak: sessionRecapStreakDTO{
			Sessions: rec.Streak.Sessions,
			Weeks:    rec.Streak.Weeks,
		},
	}

	for _, m := range rec.Muscles {
		out.Muscles = append(out.Muscles, rackedMuscleSliceDTO{
			Group:    m.Group,
			VolumeLb: m.VolumeLb,
			Sets:     m.Sets,
			Reps:     m.Reps,
			Lifts:    m.Lifts,
			Share:    m.Share,
			Trained:  m.Trained,
		})
	}
	if earned != nil {
		out.Earned = &sessionRecapEarnedDTO{
			ProgramDayID:   earned.ProgramDayID,
			ProgramDayName: earned.ProgramDayName,
			Exercises:      earned.Exercises,
		}
	}

	if !rec.Session.FinishedAt.IsZero() {
		f := rec.Session.FinishedAt.UTC().Format(time.RFC3339)
		out.Session.FinishedAt = &f
	}
	if rec.Duration > 0 {
		secs := int(rec.Duration / time.Second)
		out.DurationSeconds = &secs
	}
	if p := rec.Pace; p != nil {
		out.Pace = &sessionRecapPaceDTO{
			MedianSeconds: int(p.Median / time.Second),
			DeltaPct:      p.DeltaPct,
			Rank:          p.Rank,
			Of:            p.Of,
			SampleSize:    p.SampleSize,
		}
	}
	if on := rec.Progress.PreviousPerformedOn; on != nil {
		s := on.Format(dateLayout)
		out.Progress.PreviousPerformedOn = &s
	}

	for _, l := range rec.Lifts {
		dto := sessionRecapLiftDTO{
			ExerciseID:     l.ExerciseID,
			ExerciseName:   l.ExerciseName,
			Kind:           setKind(l.IsAssistance),
			TopWeightLb:    l.TopWeightLb,
			TopReps:        l.TopReps,
			TopE1rmLb:      l.TopE1RMLb,
			SetsLogged:     l.SetsLogged,
			SetsPrescribed: l.SetsPrescribed,
			RepsLogged:     l.RepsLogged,
			RepsTargeted:   l.RepsTargeted,
			SetsBonus:      l.SetsBonus,
			VolumeLb:       l.VolumeLb,
			HitEveryTarget: l.HitEveryTarget,
			WeightDeltaLb:  l.WeightDeltaLb,
			WeightDeltaPct: l.WeightDeltaPct,
			E1rmDeltaPct:   l.E1RMDeltaPct,
		}
		if p := l.Previous; p != nil {
			dto.Previous = &sessionRecapLiftPreviousDTO{
				PerformedOn: p.PerformedOn.Format(dateLayout),
				TopWeightLb: p.TopWeightLb,
				TopReps:     p.TopReps,
				TopE1rmLb:   p.TopE1RMLb,
			}
		}
		out.Lifts = append(out.Lifts, dto)
	}
	for _, p := range rec.PRs {
		out.PRs = append(out.PRs, rackedPRToDTO(p))
	}
	for _, f := range rec.FirstTimes {
		out.FirstTimes = append(out.FirstTimes, rackedFirstTimeToDTO(f))
	}
	for _, m := range rec.Milestones {
		out.Milestones = append(out.Milestones, rackedMilestoneToDTO(m))
	}
	return out
}
