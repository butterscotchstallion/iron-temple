package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"gitea.homelab/gitadmin/iron-temple/api/internal/progression"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

func (s *Server) getHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthDTO{Status: "ok", Version: s.version, Environment: s.environment})
}

// programSummary builds the wire shape from either of the two program reads.
//
// ListProgramsRow and GetProgramRow select the same columns and sqlc emits a
// type per query, so this takes the fields rather than either row type — which
// also keeps the one place that decides "is this mine" from being two places.
func programSummary(
	id int32, name, description, progressionKind string,
	ownerID *int32, ownerName string, isShared bool, archivedAt pgtype.Timestamptz,
	viewerID int32,
) programSummaryDTO {
	return programSummaryDTO{
		ID:              id,
		Name:            name,
		Description:     description,
		ProgressionKind: progressionKind,
		OwnerID:         ownerID,
		OwnerName:       ownerName,
		// A seeded program has no owner, so this is false for everyone — which is
		// the whole of "the install's programs are nobody's to edit".
		IsMine:     ownerID != nil && *ownerID == viewerID,
		IsShared:   isShared,
		ArchivedAt: optionalTimestamptz(archivedAt),
	}
}

func (s *Server) listPrograms(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := userFrom(ctx).ID

	rows, err := s.q.ListPrograms(ctx, store.ListProgramsParams{
		UserID: userID,
		// The owner's own retired programs, behind a query flag so the common
		// case stays the short list. It cannot widen past created_by = caller,
		// so it can never surface somebody else's.
		IncludeArchived: r.URL.Query().Get("includeArchived") == "true",
	})
	if err != nil {
		internalError(w)
		return
	}
	out := make([]programSummaryDTO, 0, len(rows))
	for _, p := range rows {
		out = append(out, programSummary(
			p.ID, p.Name, p.Description, p.ProgressionKind,
			p.CreatedByUserID, p.OwnerName, p.IsShared, p.ArchivedAt, userID,
		))
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
	userID := userFrom(ctx).ID

	// Scoped, so a program the caller may not see comes back as no rows and
	// falls into the 404 below — rather than 403, which would confirm the id.
	p, err := s.q.GetProgram(ctx, store.GetProgramParams{ID: id, UserID: userID})
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
		ProgramID: id, UserID: userID,
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
			RepMin:       a.RepMin,
			RepMax:       a.RepMax,
			Equipment:    a.Equipment,
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
		programSummaryDTO: programSummary(
			p.ID, p.Name, p.Description, p.ProgressionKind,
			p.CreatedByUserID, p.OwnerName, p.IsShared, p.ArchivedAt, userID,
		),
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

// previewNextSessions prescribes every day of a program in one response.
//
// The program detail screen shows all of a program's days at once, so asking
// day by day made rendering it cost a request per day — four on the seeded
// programs — and accepting a deload cost another one per day on top. Both are
// now a single call.
//
// The layoff is measured once here rather than per day, which is the shape the
// data always had: it describes how long the LIFTER has been away, so the
// per-day endpoint necessarily gave every day the same answer.
func (s *Server) previewNextSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	programID, ok := idParam(r, "programId")
	if !ok {
		notFound(w, "program not found")
		return
	}

	userID := userFrom(ctx).ID

	// Establishes that the program exists AND that this lifter may see it before
	// the days come back empty, so a bad id and somebody else's private program
	// are both a 404 rather than a 200 with nothing in it.
	if _, err := s.q.GetProgram(ctx, store.GetProgramParams{
		ID: programID, UserID: userID,
	}); errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "program not found")
		return
	} else if err != nil {
		internalError(w)
		return
	}

	days, err := s.q.ListProgramDays(ctx, programID)
	if err != nil {
		internalError(w)
		return
	}

	lay, err := s.layoffFor(ctx, userID, r.URL.Query().Get("deload") == "true")
	if err != nil {
		internalError(w)
		return
	}

	out := prescribedSessionsDTO{
		ProgramID: programID,
		Layoff:    lay.dto(),
		Days:      make([]prescribedDayDTO, 0, len(days)),
	}
	// Sequential on purpose. prescribe() runs several queries per day against a
	// pool this request already holds a connection from, and a program has a
	// handful of days — fanning out would trade a bounded, predictable cost for
	// contention on the same pool to save milliseconds.
	for _, day := range days {
		exercises, err := s.prescribe(ctx, programID, day.ID, userID, lay)
		if err != nil {
			internalError(w)
			return
		}
		out.Days = append(out.Days, prescribedDayDTO{
			ProgramDayID:   day.ID,
			ProgramDayName: day.Name,
			Exercises:      exercises,
		})
	}

	writeJSON(w, http.StatusOK, out)
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

	// Where this lifter starts each lift, when they have said. Read once for the
	// day rather than per lift: a program day is a handful of exercises and this
	// is one small indexed read either way.
	//
	// A baseline only ever displaces the seeded starting weight, and the seed is
	// only consulted when a lift has no history at all — so this changes the
	// first session of a lift and nothing after it. That is the whole point: the
	// seeds assume a 45 lb bar, and an install whose bar is 80 could not
	// otherwise be told where to begin.
	baselines, err := s.q.ListBaselines(ctx, userID)
	if err != nil {
		return nil, err
	}
	baseline := make(map[int32]float64, len(baselines))
	for _, b := range baselines {
		baseline[b.ExerciseID] = numericToFloat(b.WeightLb)
	}

	// What this lifter's equipment can actually build, read once for the day for
	// the same reason the baselines are: every lift below needs it, and it is
	// one small read either way.
	//
	// Unlike a baseline this is consulted on EVERY session rather than only the
	// first. It decides the size of a jump, not where a lift begins — how far a
	// successful session advances, where a deload lands, and how much assistance
	// work goes up when it tops its rep range.
	//
	// A failure here is not allowed to fail the prescription. The zero GymSteps
	// means "not configured" and falls back to the constants the engine used
	// before it was shown a gym, so an unreadable rack prescribes the standard
	// one rather than 500ing a lifter out of their workout — the same rule
	// userDTO applies to the same tables.
	var gym progression.GymSteps
	if steps, err := s.q.GetGymSteps(ctx, userID); err == nil {
		gym = progression.GymSteps{
			BarLb: numericToFloat(steps.BarStepLb),
			// Stored per bell, prescribed per pair: every weight in this app is
			// the whole load, and a dumbbell lift is two bells. See 0020.
			DumbbellLb: numericToFloat(steps.DumbbellStepLb) * 2,
			// The three stacks are NOT doubled. A dumbbell lift is the only one
			// where the lifter holds two of the thing the gym is described in;
			// a pin goes in one stack and a band is one band, so what 0028
			// stores is already the whole load.
			MachineLb: numericToFloat(steps.MachineStepLb),
			CableLb:   numericToFloat(steps.CableStepLb),
			BandLb:    numericToFloat(steps.BandStepLb),
		}
	}

	// The program's per-set prescriptions, if it has any. Only Madcow does; for
	// every other program this comes back empty and each lift is a uniform block
	// of sets x reps at one weight, exactly as before.
	//
	// Read for the whole program rather than for this day because the two
	// questions it answers have different scopes: what to load today needs this
	// day's rungs, but which day decides a lift's top set needs every day's.
	ramps, err := s.setPlans(ctx, programID)
	if err != nil {
		return nil, err
	}

	out := make([]prescribedExerciseDTO, 0, len(pres))
	for _, p := range pres {
		steps := ramps.stepsFor(dayID, p.ExerciseID)

		// Where the top set's history is read from. For a ramping lift that is
		// its reference day — the one day whose ramp reaches 100% — because the
		// same lift tops at 87.5% on one day and 102.5% on another, and a history
		// that took every day's heaviest set would read that as progress and
		// regress that never happened. For everything else it is the lift's whole
		// history, which is what it has always been.
		var hist []store.ListLiftHistoryRow
		if refDay, ok := ramps.referenceDay(p.ExerciseID); ok {
			rows, err := s.q.ListLiftHistoryForDay(ctx, store.ListLiftHistoryForDayParams{
				ExerciseID: p.ExerciseID, UserID: userID, ProgramDayID: refDay,
			})
			if err != nil {
				return nil, err
			}
			// The two row types are structurally identical — the day-scoped query
			// is otherwise a copy — so this is a conversion rather than a rebuild.
			hist = make([]store.ListLiftHistoryRow, 0, len(rows))
			for _, r := range rows {
				hist = append(hist, store.ListLiftHistoryRow(r))
			}
		} else {
			hist, err = s.q.ListLiftHistory(ctx, store.ListLiftHistoryParams{
				ExerciseID: p.ExerciseID, UserID: userID,
			})
			if err != nil {
				return nil, err
			}
		}

		history := make([]progression.SessionResult, 0, len(hist))
		for _, h := range hist {
			history = append(history, progression.SessionResult{
				WeightLb: numericToFloat(h.WeightLb), Success: h.Success,
			})
		}
		start := numericToFloat(p.StartingWeightLb)
		if b, ok := baseline[p.ExerciseID]; ok {
			start = b
		}
		// What this lift can jump by, from the movement and the equipment it
		// uses. A dumbbell press moves 10 lb because that is 5 lb a bell and a
		// rack has nothing between; a barbell lift still moves 5.
		ladder := progression.LadderFor(p.ExerciseName, p.Equipment, gym)
		plan := progression.NextPlan(start, ladder, history)
		if lay.active() {
			plan = progression.ApplyLayoff(plan, lay.weeks, ladder)
		}

		// plan.WeightLb is the top set for a ramping lift and the working weight
		// for everything else; the ramp is what tells them apart. A uniform block
		// emits a flat plan rather than nothing, so every client has one shape to
		// render instead of two.
		setPlan := progression.UniformRamp(p.Sets, p.Reps, plan.WeightLb)
		if len(steps) > 0 {
			setPlan = progression.ResolveRamp(plan.WeightLb, steps, ladder)
		}
		// A prescription is a handful of rows and cannot approach int32, but the
		// clamp is written out rather than assumed so the conversion is provably
		// safe rather than merely obviously so (gosec G115).
		setCount := int32(min(len(setPlan), math.MaxInt32))
		out = append(out, prescribedExerciseDTO{
			ExerciseID:   p.ExerciseID,
			ExerciseName: p.ExerciseName,
			Kind:         exerciseKindMain,
			Sets:         setCount,
			Reps:         p.Reps,
			// The top set, for a ramping lift. It is what the percentages are of
			// and the number that moves week to week, so it is the one to show
			// beside the lift's name — the rest of the ramp is in setPlan.
			WeightLb:    plan.WeightLb,
			RestSeconds: p.RestSeconds,
			SetPlan:     setPlanDTOs(setPlan),
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
		// Assistance progresses like everything else, unless the lifter asked
		// for the gentler rule.
		//
		// Without a rep range it runs the SAME linear engine as the prescribed
		// lifts: hit your reps on every set and the weight goes up next time,
		// miss and it repeats, miss three times and it deloads. That is what
		// "increase it like the program exercises" means, and it replaces the
		// carry-forward this branch used to do — which sounded like a considered
		// default and was really the only behaviour a lifter could reach, since
		// the rep range was opt-in, off, and had no editor.
		//
		// With a range it is double progression: add reps inside the range week
		// to week, and when every set reaches the top the weight goes up and the
		// reps reset to the bottom. No deload on that path, and on a coarse grid
		// it is much the smaller weekly increase — see progression/assistance.go.
		//
		// Both queries, because the two rules ask different questions of the
		// past. ListLiftHistory gives one row per session with the top weight and
		// whether every set was completed, which is what the linear engine reads;
		// it works here because it keys on exercise and lifter and never joins
		// the prescription, so a curl's history is its own whichever day it was
		// done on. LastAssistanceSets gives one session's per-set reps, which is
		// the only thing that can answer "did EVERY set reach the top".
		lastSets, err := s.q.LastAssistanceSets(ctx, store.LastAssistanceSetsParams{
			ExerciseID: a.ExerciseID, UserID: userID,
		})
		if err != nil {
			return nil, err
		}
		var last *progression.AssistancePerformance
		// Which session that was. Zero when the lift has never been logged,
		// matching what UpdateAssistance stores for the same state, so the two
		// compare without either side special-casing "never performed".
		var lastSessionID int32
		if len(lastSets) > 0 {
			lastReps := make([]int32, 0, len(lastSets))
			for _, r := range lastSets {
				if r.ActualReps != nil {
					lastReps = append(lastReps, *r.ActualReps)
				}
			}
			last = &progression.AssistancePerformance{
				// Every row of one session's sets carries that session's weight
				// for the lift; the first is as good as any.
				WeightLb: numericToFloat(lastSets[0].WeightLb),
				Reps:     lastReps,
			}
			// Every row carries the same session for the same reason.
			lastSessionID = lastSets[0].SessionID
		}

		// The same ladder the prescribed lifts get. Topping a rep range earns
		// the smallest move the equipment allows, so a dumbbell curl goes up 10
		// on the pair and a barbell curl 5 — before this, both went up 5 and the
		// dumbbell one asked for half a bell.
		hist, err := s.q.ListLiftHistory(ctx, store.ListLiftHistoryParams{
			ExerciseID: a.ExerciseID, UserID: userID,
		})
		if err != nil {
			return nil, err
		}
		history := make([]progression.SessionResult, 0, len(hist))
		for _, h := range hist {
			history = append(history, progression.SessionResult{
				WeightLb: numericToFloat(h.WeightLb),
				Success:  h.Success,
			})
		}

		// An explicit weight edit outranks the carry-forward, until the lift is
		// next performed.
		//
		// Both answer "what weight does this start from", and before 0022 the
		// carry-forward always won: a lifter whose curl had climbed to 50 could
		// set it to 30 on the program page and be prescribed 50 forever, with
		// the screen showing 50 the whole time. The carry-forward is still right
		// — it is what stops a weight logged mid-session being forgotten — so
		// the tie is broken by who spoke last rather than by preferring one
		// source outright.
		//
		// The pin holds the session that was most recent when the weight was
		// set, so it matches exactly until a newer session exists. Performing
		// the lift spends it with no write of its own: the id simply stops being
		// the latest, the carry-forward takes over from the weight actually
		// lifted, and progression resumes from there. A lift never logged pins
		// against 0, which is what the query stores for "no session yet".
		//
		// Dropping history along with last is deliberate. A weight the lifter
		// chose is a fresh start, not the next rung from the old one: keeping
		// the trailing misses at 50 would let a lift deload off a 30 nobody has
		// failed at yet, which is the opposite of what setting it meant.
		if pin := a.WeightSetAfterSessionID; pin != nil && *pin == lastSessionID {
			last, history = nil, nil
		}

		ladder := progression.LadderFor(a.ExerciseName, a.Equipment, gym)
		plan := progression.NextAssistance(
			numericToFloat(a.WeightLb), derefInt32(a.RepMin), derefInt32(a.RepMax),
			last, history, ladder,
		)
		weight := plan.WeightLb
		previous := plan.PreviousLb

		// The rep target is the bottom of the range when there is one: a set is
		// complete at the bottom and the weight moves at the top, which is what
		// keeps "finished the session" and "earned the increase" separate. With
		// no range the stored reps stand.
		reps := a.Reps
		if plan.TargetReps > 0 {
			reps = plan.TargetReps
		}

		// A layoff reaches assistance too. Three weeks out of the gym cost the
		// curl what they cost the squat, and a lifter who agreed to ease back in
		// did not mean "except the accessories".
		//
		// Through ApplyLayoff rather than by overwriting the weight, which is
		// what this did while assistance could not deload. Now that the unranged
		// path is the linear engine it can, and the two cuts must not compound:
		// they are two answers to "how light should this be", so the deeper wins
		// outright and the shallower is a no-op. Overwriting would let a week off
		// after a stall quietly UNDO the deload by putting the weight back up.
		//
		// The `previous > 0` guard this used to apply here now lives inside
		// ApplyLayoff, as PreviousLb <= 0 — a stored fallback weight is what to
		// use the first time, not something to detrain off. It is checked there
		// rather than here because it is a fact about the plan, not about this
		// call site, and because the status it used to be inferred from
		// (StatusStart) is not the only way to have no history: a ranged
		// accessory never performed reports StatusFixed with PreviousLb 0.
		layoffState := progression.Plan{
			WeightLb:   weight,
			Status:     plan.Status,
			PreviousLb: previous,
		}
		if lay.active() {
			layoffState = progression.ApplyLayoff(layoffState, lay.weeks, ladder)
		}
		weight = layoffState.WeightLb
		layoffPct := layoffState.LayoffPct

		// A layoff cut outranks a rep-range advance in the label as well as in
		// the number: a lifter looking at a weight that just went down wants to
		// be told why it went down.
		status := string(layoffState.Status)
		out = append(out, prescribedExerciseDTO{
			ExerciseID:   a.ExerciseID,
			ExerciseName: a.ExerciseName,
			Kind:         exerciseKindAssistance,
			Sets:         a.Sets,
			Reps:         reps,
			WeightLb:     weight,
			RestSeconds:  a.RestSeconds,
			RepMin:       a.RepMin,
			RepMax:       a.RepMax,
			// Assistance never ramps — it is a block of the same work at the same
			// weight — but it still emits a plan, so createSession and the client
			// have one shape to read for every lift in the session.
			SetPlan: setPlanDTOs(progression.UniformRamp(a.Sets, reps, weight)),
			Progression: progressionInfoDTO{
				Status:               status,
				FailureCount:         plan.FailureCount,
				FailuresBeforeDeload: progression.FailuresBeforeDeload,
				PreviousWeightLb:     previous,
				LayoffPct:            layoffPct,
			},
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Per-set prescriptions (ramps)
// ---------------------------------------------------------------------------

// rampIndex is one program's per-set prescriptions, arranged for the two
// questions prescribe() asks of them: what are today's rungs for this lift, and
// which day decides this lift's top set.
//
// Empty for every program but Madcow, and every method below answers "no ramp"
// for an empty index — which is what keeps the linear programs on exactly the
// path they were on before ramps existed.
type rampIndex struct {
	byDayAndLift map[[2]int32][]progression.RampStep
	reference    map[int32]int32
}

func (r rampIndex) stepsFor(dayID, exerciseID int32) []progression.RampStep {
	return r.byDayAndLift[[2]int32{dayID, exerciseID}]
}

// referenceDay returns the day whose ramp reaches this lift's top set. Absent
// for a lift with no ramp, and — deliberately — for a ramp that names no 100%
// set at all: a prescription that never establishes a top set has nothing for
// the engine to read, and falling back to the lift's whole history is a better
// answer than picking a day arbitrarily.
func (r rampIndex) referenceDay(exerciseID int32) (int32, bool) {
	day, ok := r.reference[exerciseID]
	return day, ok
}

func (s *Server) setPlans(ctx context.Context, programID int32) (rampIndex, error) {
	rows, err := s.q.ListSetPlansByProgram(ctx, programID)
	if err != nil {
		return rampIndex{}, err
	}
	idx := rampIndex{
		byDayAndLift: make(map[[2]int32][]progression.RampStep),
		reference:    make(map[int32]int32),
	}
	for _, row := range rows {
		key := [2]int32{row.ProgramDayID, row.ExerciseID}
		idx.byDayAndLift[key] = append(idx.byDayAndLift[key], progression.RampStep{
			SetNumber: row.SetNumber,
			Reps:      row.Reps,
			PctOfTop:  numericToFloat(row.PctOfTop),
		})
	}
	for key, steps := range idx.byDayAndLift {
		if progression.IsReferenceDay(steps) {
			idx.reference[key[1]] = key[0]
		}
	}
	return idx, nil
}

func setPlanDTOs(sets []progression.RampSet) []prescribedSetDTO {
	out := make([]prescribedSetDTO, 0, len(sets))
	for _, s := range sets {
		out = append(out, prescribedSetDTO{
			SetNumber: s.SetNumber,
			Reps:      s.Reps,
			WeightLb:  s.WeightLb,
		})
	}
	return out
}
