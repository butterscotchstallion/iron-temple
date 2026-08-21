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

// getRacked serves the recap for the week, month or year containing the `on`
// query parameter, defaulting to the month in progress.
func (s *Server) getRacked(w http.ResponseWriter, r *http.Request) {
	kind := racked.PeriodMonth
	if v := r.URL.Query().Get("period"); v != "" {
		parsed, ok := racked.ParsePeriodKind(v)
		if !ok {
			badRequest(w, "period must be week, month or year")
			return
		}
		kind = parsed
	}

	// Both the period and the point it is measured to come from this one
	// reading — see reportToday.
	today := s.reportToday()
	on := today
	if v := r.URL.Query().Get("on"); v != "" {
		parsed, err := time.Parse(dateLayout, v)
		if err != nil {
			badRequest(w, "on must be a date in YYYY-MM-DD form")
			return
		}
		on = parsed
	}

	report, err := s.buildRacked(r.Context(), userFrom(r.Context()).ID, kind, on, today)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, rackedReportToDTO(report))
}

// buildRacked gathers a period's rows and reduces them to a report.
//
// Five reads, none of them clever: the period's sets, the preceding period's
// sets for the comparison, the two baselines that let a maximum inside the
// period be recognised as a record, and the period's weigh-ins. Everything that
// looks like a statistic happens in internal/racked, over plain Go values.
//
// It is exported through the Server rather than inlined into the handler
// because the recap email needs exactly this, for a user with no request in
// flight.
func (s *Server) buildRacked(
	ctx context.Context, userID int32, kind racked.PeriodKind, on, asOf time.Time,
) (racked.Report, error) {
	start, end := racked.Bounds(kind, on)
	prevStart, prevEnd := racked.PreviousBounds(kind, on)

	current, err := s.rackedSets(ctx, userID, start, end)
	if err != nil {
		return racked.Report{}, err
	}
	previous, err := s.rackedSets(ctx, userID, prevStart, prevEnd)
	if err != nil {
		return racked.Report{}, err
	}
	baseline, err := s.rackedBaseline(ctx, userID, start)
	if err != nil {
		return racked.Report{}, err
	}
	weighIns, err := s.rackedWeighIns(ctx, userID, start, end)
	if err != nil {
		return racked.Report{}, err
	}
	days, programStarted, err := s.rackedProgramDays(ctx, userID)
	if err != nil {
		return racked.Report{}, err
	}

	// asOf is the day the recap is drawn up on, which for the default view falls
	// inside the period: racked then measures its rates over the days that have
	// actually happened and cuts the preceding period to match. It is passed in
	// rather than read here so that it and `on` are the same reading of the
	// clock.
	return racked.Build(racked.Input{
		Kind:           kind,
		Start:          start,
		End:            end,
		AsOf:           asOf,
		Loc:            s.reportLocation(),
		Sets:           current,
		WeighIns:       weighIns,
		PreviousSets:   previous,
		PreviousStart:  prevStart,
		Baseline:       baseline,
		ProgramDays:    days,
		ProgramStarted: programStarted,
	}), nil
}

func (s *Server) rackedSets(
	ctx context.Context, userID int32, start, end time.Time,
) ([]racked.Set, error) {
	rows, err := s.q.RackedPeriodSets(ctx, store.RackedPeriodSetsParams{
		UserID:  userID,
		StartOn: pgtype.Date{Time: start, Valid: true},
		EndOn:   pgtype.Date{Time: end, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := make([]racked.Set, 0, len(rows))
	for _, row := range rows {
		reps := 0
		if row.ActualReps != nil {
			reps = int(*row.ActualReps)
		}
		out = append(out, racked.Set{
			SessionID:      row.SessionID,
			PerformedOn:    row.PerformedOn.Time,
			StartedAt:      row.CreatedAt.Time,
			FinishedAt:     finishedAt(row.FinishedAt),
			ProgramDayName: row.ProgramDayName,
			ExerciseID:     row.ExerciseID,
			ExerciseName:   row.ExerciseName,
			Reps:           reps,
			WeightLb:       numericToFloat(row.WeightLb),
			Completed:      row.Completed,
			IsAssistance:   row.IsAssistance,
		})
	}
	return out, nil
}

// rackedWeighIns reads the period's recorded bodyweights. An empty result is the
// normal case and not an absence to paper over: most lifters never fill the box
// in, and racked reports no bodyweight section at all for them.
func (s *Server) rackedWeighIns(
	ctx context.Context, userID int32, start, end time.Time,
) ([]racked.WeighIn, error) {
	rows, err := s.q.RackedWeighIns(ctx, store.RackedWeighInsParams{
		UserID:  userID,
		StartOn: pgtype.Date{Time: start, Valid: true},
		EndOn:   pgtype.Date{Time: end, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := make([]racked.WeighIn, 0, len(rows))
	for _, row := range rows {
		out = append(out, racked.WeighIn{
			PerformedOn: row.PerformedOn.Time,
			WeightLb:    numericToFloat(row.BodyweightLb),
		})
	}
	return out, nil
}

// finishedAt flattens a nullable finish stamp to the zero time, which is how
// racked spells "this session was never finished".
func finishedAt(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

func (s *Server) rackedBaseline(
	ctx context.Context, userID int32, start time.Time,
) (racked.Baseline, error) {
	startDate := pgtype.Date{Time: start, Valid: true}

	volume, err := s.q.RackedVolumeBefore(ctx, store.RackedVolumeBeforeParams{
		UserID: userID, StartOn: startDate,
	})
	if err != nil {
		return racked.Baseline{}, err
	}
	rows, err := s.q.RackedExerciseBaseline(ctx, store.RackedExerciseBaselineParams{
		UserID: userID, StartOn: startDate,
	})
	if err != nil {
		return racked.Baseline{}, err
	}

	base := racked.Baseline{
		VolumeLb:   numericToFloat(volume),
		BestWeight: make(map[int32]float64, len(rows)),
		BestE1RM:   make(map[int32]float64, len(rows)),
	}
	for _, row := range rows {
		base.BestWeight[row.ExerciseID] = numericToFloat(row.BestWeightLb)
		base.BestE1RM[row.ExerciseID] = numericToFloat(row.BestE1rmLb)
	}
	return base, nil
}

// rackedProgramDays returns the days of the lifter's current program and when
// that program came into being, which together are what attendance measures
// against. A lifter with no current program is not an error: the recap reports
// an attendance basis of "none" and says nothing.
//
// The creation date travels with the days because a program is only evidence
// about periods it existed for — see attendance in internal/racked.
func (s *Server) rackedProgramDays(
	ctx context.Context, userID int32,
) ([]racked.ProgramDay, time.Time, error) {
	user, err := s.q.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, time.Time{}, nil
		}
		return nil, time.Time{}, err
	}
	if user.CurrentProgramID == nil {
		return nil, time.Time{}, nil
	}
	rows, err := s.q.ListProgramDays(ctx, *user.CurrentProgramID)
	if err != nil {
		return nil, time.Time{}, err
	}
	started, err := s.q.RackedProgramStart(ctx, *user.CurrentProgramID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, time.Time{}, err
	}

	out := make([]racked.ProgramDay, 0, len(rows))
	for _, row := range rows {
		day := racked.ProgramDay{Name: row.Name}
		if row.Weekday != nil {
			wd := int(*row.Weekday)
			day.Weekday = &wd
		}
		out = append(out, day)
	}

	// Compared against period dates, which are UTC midnights, so the stamp is
	// reduced to the day it fell on.
	var startedOn time.Time
	if started.Valid {
		t := started.Time.In(s.reportLocation())
		startedOn = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	return out, startedOn, nil
}

// ---- wire conversion ----

func rackedWorkToDTO(w racked.Work) rackedWorkDTO {
	return rackedWorkDTO{
		VolumeLb: w.VolumeLb,
		Sets:     w.Sets,
		Reps:     w.Reps,
		Lifts:    w.Lifts,
		Share:    w.Share,
	}
}

func rackedReportToDTO(rep racked.Report) rackedReportDTO {
	out := rackedReportDTO{
		Period: rackedPeriodDTO{
			Kind:       string(rep.Period.Kind),
			Start:      rep.Period.Start.Format(dateLayout),
			End:        rep.Period.End.Format(dateLayout),
			Label:      rep.Period.Label,
			InProgress: rep.Period.InProgress,
		},
		Totals: rackedTotalsDTO{
			VolumeLb: rep.Totals.VolumeLb,
			Sessions: rep.Totals.Sessions,
			Sets:     rep.Totals.Sets,
			Reps:     rep.Totals.Reps,
		},
		Comparison: rackedComparisonDTO{
			Count:  rep.Comparison.Count,
			Label:  rep.Comparison.Label,
			UnitLb: rep.Comparison.UnitLb,
		},
		Split: rackedSplitDTO{
			Main:       rackedWorkToDTO(rep.Split.Main),
			Assistance: rackedWorkToDTO(rep.Split.Assistance),
		},
		Lifts:       make([]rackedLiftSliceDTO, 0, len(rep.Lifts)),
		Series:      make([]rackedSeriesDTO, 0, len(rep.Series)),
		Days:        make([]rackedDayVolumeDTO, 0, len(rep.Days)),
		Weekdays:    rep.Weekdays,
		BestWeekday: rep.BestWeekday,
		Hours:       rep.Hours,
		PeakHour:    rep.PeakHour,
		HourLabel:   rep.HourLabel,
		Streak: rackedStreakDTO{
			LongestWeeks: rep.Streak.LongestWeeks,
			CurrentWeeks: rep.Streak.CurrentWeeks,
		},
		Attendance: rackedAttendanceDTO{
			Basis:           string(rep.Attendance.Basis),
			Expected:        rep.Attendance.Expected,
			Actual:          rep.Attendance.Actual,
			Rate:            rep.Attendance.Rate,
			SessionsPerWeek: rep.Attendance.SessionsPerWeek,
		},
		PRs:        make([]rackedPRDTO, 0, len(rep.PRs)),
		Milestones: make([]rackedMilestoneDTO, 0, len(rep.Milestones)),
		Deloads:    make([]rackedDeloadDTO, 0, len(rep.Deloads)),
		Archetype: rackedArchetypeDTO{
			Name:        rep.Archetype.Name,
			Description: rep.Archetype.Description,
		},
	}

	if c := rep.Change; c != nil {
		out.Change = &rackedChangeDTO{
			VolumeLb:    c.VolumeLb,
			VolumePct:   c.VolumePct,
			Sessions:    c.Sessions,
			SessionsPct: c.SessionsPct,
		}
	}
	if b := rep.Bodyweight; b != nil {
		points := make([]rackedWeighInDTO, 0, len(b.Points))
		for _, p := range b.Points {
			points = append(points, rackedWeighInDTO{
				PerformedOn: p.PerformedOn.Format(dateLayout),
				WeightLb:    p.WeightLb,
			})
		}
		out.Bodyweight = &rackedBodyweightDTO{
			Points:    points,
			StartLb:   b.StartLb,
			EndLb:     b.EndLb,
			LowLb:     b.LowLb,
			HighLb:    b.HighLb,
			ChangeLb:  b.ChangeLb,
			ChangePct: b.ChangePct,
		}
	}
	for _, l := range rep.Lifts {
		out.Lifts = append(out.Lifts, rackedLiftSliceDTO{
			ExerciseID:   l.ExerciseID,
			ExerciseName: l.ExerciseName,
			VolumeLb:     l.VolumeLb,
			Sets:         l.Sets,
			Reps:         l.Reps,
			Share:        l.Share,
			IsAssistance: l.IsAssistance,
		})
	}
	for _, s := range rep.Series {
		points := make([]rackedSeriesPointDTO, 0, len(s.Points))
		for _, p := range s.Points {
			points = append(points, rackedSeriesPointDTO{
				PerformedOn: p.PerformedOn.Format(dateLayout),
				TopWeightLb: p.TopWeightLb,
				E1RMLb:      p.E1RMLb,
			})
		}
		out.Series = append(out.Series, rackedSeriesDTO{
			ExerciseID:   s.ExerciseID,
			ExerciseName: s.ExerciseName,
			IsAssistance: s.IsAssistance,
			Points:       points,
		})
	}
	if m := rep.MostImproved; m != nil {
		out.MostImproved = &rackedImprovementDTO{
			ExerciseID:   m.ExerciseID,
			ExerciseName: m.ExerciseName,
			FromLb:       m.FromLb,
			ToLb:         m.ToLb,
			GainLb:       m.GainLb,
			GainPct:      m.GainPct,
		}
	}
	for _, d := range rep.Days {
		out.Days = append(out.Days, rackedDayVolumeDTO{
			Date:     d.Date.Format(dateLayout),
			VolumeLb: d.VolumeLb,
			Sessions: d.Sessions,
		})
	}
	for _, p := range rep.PRs {
		out.PRs = append(out.PRs, rackedPRDTO{
			Kind:         string(p.Kind),
			PerformedOn:  p.PerformedOn.Format(dateLayout),
			ExerciseID:   p.ExerciseID,
			ExerciseName: p.ExerciseName,
			WeightLb:     p.WeightLb,
			Reps:         p.Reps,
			ValueLb:      p.ValueLb,
			PreviousLb:   p.PreviousLb,
		})
	}
	for _, m := range rep.Milestones {
		out.Milestones = append(out.Milestones, rackedMilestoneDTO{
			Kind:         string(m.Kind),
			PerformedOn:  m.PerformedOn.Format(dateLayout),
			Label:        m.Label,
			ValueLb:      m.ValueLb,
			ExerciseID:   m.ExerciseID,
			ExerciseName: m.ExerciseName,
		})
	}
	if h := rep.HeaviestSet; h != nil {
		out.HeaviestSet = &rackedSetHighlightDTO{
			PerformedOn:  h.PerformedOn.Format(dateLayout),
			ExerciseID:   h.ExerciseID,
			ExerciseName: h.ExerciseName,
			WeightLb:     h.WeightLb,
			Reps:         h.Reps,
		}
	}
	if f := rep.FastestSession; f != nil {
		out.FastestSession = &rackedSessionHighlightDTO{
			SessionID:       f.SessionID,
			PerformedOn:     f.PerformedOn.Format(dateLayout),
			ProgramDayName:  f.ProgramDayName,
			DurationSeconds: int(f.Duration.Seconds()),
			VolumeLb:        f.VolumeLb,
			Sets:            f.Sets,
		}
	}
	for _, d := range rep.Deloads {
		dto := rackedDeloadDTO{
			ExerciseID:   d.ExerciseID,
			ExerciseName: d.ExerciseName,
			PerformedOn:  d.PerformedOn.Format(dateLayout),
			FromLb:       d.FromLb,
			ToLb:         d.ToLb,
			Recovered:    d.Recovered,
		}
		if d.Recovered {
			on := d.RecoveredOn.Format(dateLayout)
			dto.RecoveredOn = &on
		}
		out.Deloads = append(out.Deloads, dto)
	}
	return out
}
