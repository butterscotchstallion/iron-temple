package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"gitea.homelab/gitadmin/iron-temple/api/internal/activity"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Generated training activity, for looking at the screens built for a shared gym
// without waiting three months for four people to fill them.
//
// # WHO CAN REACH THIS
//
// Nobody but the install's owner. These handlers are mounted inside the /admin
// subtree in Router, which applies requireAdmin to everything under it — so they
// inherit the gate from where they sit rather than from anybody remembering to
// add it, which is the property that subtree's own comment exists to guarantee.
//
// # WHY IT RUNS IN-PROCESS
//
// The alternative was an external tool driving the public API as a client, which
// would exercise more of the stack. In-process wins on the thing that actually
// matters for generated data being TRUSTWORTHY: it calls s.prescribe, so every
// weight comes from the real progression engine, and it reuses allowedReactions
// and maxCommentBody rather than holding second copies that could drift. Data
// generated against a second implementation of the rules is data that can be
// wrong in exactly the ways the app is being checked for.
//
// Where a rule lives only in a handler, it is applied here explicitly and the
// handler is named. There are two: no reacting to your own session, and a comment
// must be non-blank and within the cap.
//
// # WHAT IT DELIBERATELY DOES NOT DO
//
// Generated accounts carry NO MARKER. Nothing in the schema and nothing on the
// wire distinguishes them from an account somebody typed in, which is a decision
// the install's owner made rather than an oversight. The consequence is that
// teardown cannot look them up — it re-derives the roster that named them, which
// is why activity.Roster is required to be stable and is tested as such.

const (
	// Bounds on one request. Not security — the caller is already the owner — but
	// a backfill is a long synchronous loop, and these are what keep a typo in a
	// number from turning into a request that never returns.
	maxBackfillWeeks = 26
	minActivityTick  = 5 * time.Second
	maxActivityTick  = time.Hour

	// A generated account's password. It is never used: nothing signs in as these
	// lifters, because the runner writes as them directly. It exists because the
	// column is NOT NULL, and it is a fixed string rather than a random one so
	// that a re-run is idempotent rather than locking out an account it made
	// earlier. Safe only because this is dev tooling on a box whose owner asked
	// for it; if these accounts ever needed to be signed into, this is the line
	// that has to change first.
	generatedPassword = "generated-activity-not-for-sign-in"
)

// activityRunner is the live loop's state.
//
// One per server. Guarded by a mutex rather than run through a channel because
// every operation on it is a short read or a flag flip from an HTTP handler, and a
// goroutine to serialise those would be more machinery than the thing it protects.
type activityRunner struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
	tick    time.Duration
	lifters int
	started time.Time
	// actions is how many things the loop has done since it started, and last is
	// the most recent, so the admin screen can show it is alive rather than merely
	// flagged as running.
	actions int
	last    string
	// gen identifies which loop owns the fields above, and exists because starting
	// replaces rather than refuses. A restart cancels the old loop and installs a
	// new one immediately, so the old goroutine wakes up some time LATER to wind
	// down — by which point the flag and the cancel belong to its successor. gen
	// is what lets it tell. See finish.
	gen int
}

func (a *activityRunner) snapshot() (running bool, tick time.Duration, lifters, actions int, started time.Time, last string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running, a.tick, a.lifters, a.actions, a.started, a.last
}

func (a *activityRunner) note(what string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions++
	a.last = what
}

// stop halts the loop if one is running. Safe to call when none is.
func (a *activityRunner) stop() {
	a.mu.Lock()
	cancel := a.cancel
	a.cancel = nil
	a.running = false
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// begin claims the slot for a new loop and returns its generation.
func (a *activityRunner) begin(cancel context.CancelFunc, tick time.Duration, lifters int) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.gen++
	a.cancel = cancel
	a.running = true
	a.tick = tick
	a.lifters = lifters
	a.started = time.Now()
	a.actions = 0
	a.last = ""
	return a.gen
}

// finish releases a loop's slot as it exits — but only if it still holds it.
//
// Every exit from runActivity comes through here, including the early ones. The
// flag is raised by the handler before the goroutine is even scheduled, so a loop
// that fails to start would otherwise leave the status reporting a loop that does
// not exist, recoverable only by pressing Stop on nothing.
//
// The generation check is what keeps that from becoming a worse bug than the one
// it fixes. Starting replaces rather than refuses, so a restart cancels this loop
// and installs its successor at once; this goroutine then wakes some time later to
// wind down. Clearing unconditionally at that point would switch the NEW loop's
// flag off and fire its cancel, killing a loop the admin had just started.
func (a *activityRunner) finish(gen int, cancel context.CancelFunc) {
	// Released whoever owns the slot: this loop's context is done or about to be,
	// and a CancelFunc that is never called leaks it.
	cancel()

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.gen != gen {
		return
	}
	a.running = false
	a.cancel = nil
}

// ---- requests and handlers ----

type backfillRequest struct {
	Lifters int `json:"lifters"`
	Weeks   int `json:"weeks"`
}

type startActivityRequest struct {
	Lifters     int `json:"lifters"`
	TickSeconds int `json:"tickSeconds"`
}

// getActivityStatus reports what the runner is doing, and what a roster would be.
func (s *Server) getActivityStatus(w http.ResponseWriter, r *http.Request) {
	running, tick, lifters, actions, started, last := s.activity.snapshot()

	dto := activityStatusDTO{
		Running:     running,
		Lifters:     lifters,
		Actions:     actions,
		LastAction:  last,
		MaxLifters:  activity.MaxRoster,
		MaxWeeks:    maxBackfillWeeks,
		TickSeconds: int(tick / time.Second),
		// The whole roster, not the count a loop happens to be using: teardown
		// deletes by name across all of it, so all of it is what an operator needs
		// to see before confirming one.
		Roster: activity.Usernames(activity.MaxRoster),
	}
	if running && !started.IsZero() {
		dto.StartedAt = started.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, dto)
}

// postActivityBackfill generates history.
//
// Synchronous, and that is a choice rather than a limitation: a backfill of a
// handful of lifters over a few months is seconds of work, and returning a job id
// to poll would be more moving parts than the thing it reports on. The bounds
// above are what keep it that way.
func (s *Server) postActivityBackfill(w http.ResponseWriter, r *http.Request) {
	var req backfillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Lifters < 1 || req.Lifters > activity.MaxRoster {
		badRequest(w, "lifters must be between 1 and 8")
		return
	}
	if req.Weeks < 1 || req.Weeks > maxBackfillWeeks {
		badRequest(w, "weeks must be between 1 and 26")
		return
	}

	summary, err := s.backfillActivity(r.Context(), req.Lifters, req.Weeks)
	if err != nil {
		log.Printf("activity backfill: %v", err)
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// postActivityStart begins the live loop.
func (s *Server) postActivityStart(w http.ResponseWriter, r *http.Request) {
	var req startActivityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Lifters < 1 || req.Lifters > activity.MaxRoster {
		badRequest(w, "lifters must be between 1 and 8")
		return
	}
	tick := time.Duration(req.TickSeconds) * time.Second
	if tick < minActivityTick || tick > maxActivityTick {
		badRequest(w, "tickSeconds must be between 5 and 3600")
		return
	}

	// Replaces any loop already running rather than refusing. Starting twice is
	// what an admin does when they want a different tick, and a 409 would make
	// them stop it first for no reason.
	s.activity.stop()

	// Deliberately NOT the request's context: that is cancelled the moment this
	// response is written, and a loop tied to it would stop before the admin's
	// browser had finished rendering the button they just pressed. The loop's
	// lifetime is the server's, and the only things that end it are stop() and
	// the process exiting.
	ctx, cancel := context.WithCancel(context.Background())
	gen := s.activity.begin(cancel, tick, req.Lifters)

	go s.runActivity(ctx, gen, cancel, req.Lifters, tick)
	w.WriteHeader(http.StatusNoContent)
}

// postActivityStop halts it. 204 whether or not one was running — the caller asked
// for a state, and it holds either way.
func (s *Server) postActivityStop(w http.ResponseWriter, _ *http.Request) {
	s.activity.stop()
	w.WriteHeader(http.StatusNoContent)
}

// deleteActivity removes the generated accounts and everything that cascades from
// them.
func (s *Server) deleteActivity(w http.ResponseWriter, r *http.Request) {
	// Stopped first. A loop still writing while its accounts are deleted would
	// spend the next tick logging errors about rows that have gone.
	s.activity.stop()

	removed, err := s.removeGeneratedLifters(r.Context())
	if err != nil {
		log.Printf("activity teardown: %v", err)
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, activityTeardownDTO{Removed: removed})
}

// removeGeneratedLifters deletes the generated accounts, and ONLY those.
//
// The roster gives candidates; the password hash decides. Both steps matter:
//
// The whole roster is offered as candidates, not the count some previous backfill
// happened to use — the caller may not remember it, and Roster is prefix-stable so
// asking for all of it is a superset of anything ever created.
//
// Then each candidate's hash is verified against the fixed password every generated
// account is made with. That is what makes this ORIGIN-scoped rather than
// name-scoped, and the difference is somebody's training history: the personas are
// ordinary household names on purpose, so an account the owner created by hand as
// "mara.quinn" used to be indistinguishable from a generated one and would have
// been deleted with everything that lifter had logged. They chose their own
// password, so their hash does not verify, so they survive.
//
// No marker was added to get here. The hash is evidence that was already in the
// database, costs no column and appears on no wire — see
// ListGeneratedActivityCandidates for why reading it is the one sanctioned
// exception to the rule in users.sql.
func (s *Server) removeGeneratedLifters(ctx context.Context) (int64, error) {
	candidates, err := s.q.ListGeneratedActivityCandidates(ctx, activity.Usernames(activity.MaxRoster))
	if err != nil {
		return 0, err
	}

	ids := make([]int32, 0, len(candidates))
	for _, candidate := range candidates {
		// needsRehash is ignored deliberately: these accounts are never signed in
		// to, so an out-of-date cost parameter on one is of no consequence — and
		// rehashing something that is about to be deleted would be absurd.
		ok, _ := s.hasher.Verify(generatedPassword, candidate.PasswordHash)
		if !ok {
			// A real lifter who happens to share the name. Logged rather than
			// silently skipped: it is the one outcome here an operator would want
			// to know about, because it means a roster name is in use for real.
			log.Printf("activity teardown: keeping %q — not a generated account", candidate.Username)
			continue
		}
		ids = append(ids, candidate.ID)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return s.q.DeleteGeneratedActivityUsers(ctx, ids)
}

// ---- the runner ----

// simulatedLifter is one persona joined to the account and program standing in
// for it.
type simulatedLifter struct {
	persona activity.Persona
	userID  int32
	// days are the program days they train, each with the weekday it falls on.
	days []store.ProgramDay
}

// backfillActivity is the whole generation pass.
func (s *Server) backfillActivity(ctx context.Context, lifters, weeks int) (activitySummaryDTO, error) {
	var summary activitySummaryDTO

	// Seeded from the request's shape rather than the clock, so the same backfill
	// twice over a clean database produces the same history — which is what makes
	// a screen that looks wrong reproducible.
	rng := simulationRNG(int64(lifters), int64(weeks))

	people, created, err := s.ensureGeneratedLifters(ctx, lifters)
	if err != nil {
		return summary, err
	}
	summary.Accounts = created

	today := s.reportToday()
	// Oldest first, which is load-bearing: every weight comes from s.prescribe,
	// which reads the lifter's history, so a session generated out of order would
	// be prescribed from a future it has not had yet. Walking forwards means the
	// progression engine sees exactly what it would have seen at the time.
	for w := weeks - 1; w >= 0; w-- {
		weekStart := today.AddDate(0, 0, -7*w)
		for i := range people {
			person := &people[i]
			for _, day := range person.days {
				// A day with no weekday has no date to land on. scheduleProgram
				// fills blanks, so this is the case where the owner has a program
				// day they have deliberately left unscheduled.
				if day.Weekday == nil {
					continue
				}
				on := weekdayIn(weekStart, int(*day.Weekday))
				if on.After(today) {
					continue
				}
				if !person.persona.Trains(rng) {
					continue
				}
				logged, err := s.generateSession(ctx, person, day, on, rng)
				if err != nil {
					return summary, err
				}
				if logged {
					summary.Sessions++
				}
			}
		}
	}

	reactions, comments, err := s.generateRecognition(ctx, people, rng)
	if err != nil {
		return summary, err
	}
	summary.Reactions = reactions
	summary.Comments = comments
	return summary, nil
}

// ensureGeneratedLifters creates the roster's accounts if they are absent, and
// returns every one of them with the program days they train.
//
// Re-runnable: an account that already exists is adopted rather than recreated,
// so a second backfill adds history to the lifters a first one made instead of
// failing on a taken username.
func (s *Server) ensureGeneratedLifters(
	ctx context.Context, lifters int,
) ([]simulatedLifter, int, error) {
	programs, err := s.q.ListPrograms(ctx)
	if err != nil {
		return nil, 0, err
	}
	if len(programs) == 0 {
		return nil, 0, errors.New("no programs seeded, so there is nothing to train")
	}

	roster := activity.Roster(lifters)
	out := make([]simulatedLifter, 0, len(roster))
	var created int

	for i, persona := range roster {
		userID, isNew, err := s.ensureGeneratedAccount(ctx, persona)
		if err != nil {
			return nil, 0, err
		}
		if isNew {
			created++
		}

		// One program each where there are enough to go round, which keeps their
		// schedules from colliding — program_days is shared, so two lifters on one
		// program necessarily train the same weekdays.
		program := programs[i%len(programs)]
		days, err := s.scheduleProgram(ctx, program.ID, i)
		if err != nil {
			return nil, 0, err
		}
		if _, err := s.q.UpdateUserProfile(ctx, store.UpdateUserProfileParams{
			ID:               userID,
			CurrentProgramID: &program.ID,
		}); err != nil {
			return nil, 0, err
		}

		out = append(out, simulatedLifter{persona: persona, userID: userID, days: days})
	}
	return out, created, nil
}

// ensureGeneratedAccount finds or makes one account.
func (s *Server) ensureGeneratedAccount(
	ctx context.Context, persona activity.Persona,
) (userID int32, created bool, err error) {
	id, err := s.q.FindUserIDByUsername(ctx, persona.Username)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, err
	}

	hash, err := s.hasher.Hash(generatedPassword)
	if err != nil {
		return 0, false, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	user, err := qtx.CreateUser(ctx, store.CreateUserParams{
		Username:    persona.Username,
		DisplayName: persona.DisplayName,
		// Never an admin: users_single_admin_idx permits exactly one and the
		// install's owner holds it. Also what keeps the NOT is_admin guard on the
		// teardown queries from ever having to refuse one of these.
		IsAdmin: false,
		// The fixed password. Nothing signs in with it — see generatedPassword —
		// and its hash is what lets a teardown prove which accounts it made, so
		// this line is load-bearing for more than the NOT NULL constraint.
		PasswordHash: hash,
		// False, unlike an account the admin area creates. That flag exists to
		// force a lifter to replace a password somebody else chose, and there is
		// nobody here to do the replacing — leaving it set would lock every
		// generated account out of the app it is meant to populate.
		MustChangePassword: false,
	})
	if err != nil {
		return 0, false, err
	}
	// Same transaction as the account, exactly as createUser does it: an account
	// either owns a rack or does not exist. Without this the generated lifter gets
	// bar-only plate maths and their weights round differently from everybody
	// else's.
	if err := qtx.SeedDefaultPlates(ctx, user.ID); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	return user.ID, true, nil
}

// scheduleProgram gives a program's days weekdays, so attendance has a basis.
//
// Only fills blanks — see SetProgramDayWeekdayIfUnset. program_days is shared
// across the install, so this must never rearrange a schedule the owner set.
func (s *Server) scheduleProgram(
	ctx context.Context, programID int32, offset int,
) ([]store.ProgramDay, error) {
	days, err := s.q.ListProgramDays(ctx, programID)
	if err != nil {
		return nil, err
	}
	for i, day := range days {
		// Monday, Wednesday, Friday and outwards, shifted per lifter so two
		// programs do not land every session on the same day of the week.
		//
		// Indexed into weekdayValues rather than computed and narrowed, so the
		// result is provably one of the seven the column admits. offset is a roster
		// index and i a slice index, so neither is negative and the modulo cannot
		// produce one either.
		weekday := weekdayValues[(1+2*i+offset)%7]
		if err := s.q.SetProgramDayWeekdayIfUnset(ctx, store.SetProgramDayWeekdayIfUnsetParams{
			ID:      day.ID,
			Weekday: &weekday,
		}); err != nil {
			return nil, err
		}
	}
	// Re-read, so the returned days carry whatever weekday actually holds — the
	// one this just set, or the one the owner had already chosen.
	return s.q.ListProgramDays(ctx, programID)
}

// generateSession materialises one session, logs it, finishes it and sets its
// clock. Reports whether anything was logged.
func (s *Server) generateSession(
	ctx context.Context,
	person *simulatedLifter,
	day store.ProgramDay,
	on time.Time,
	rng *rand.Rand,
) (bool, error) {
	// The real engine, so every weight is one the app would actually have
	// prescribed — including the stalls and the deloads a fallible persona earns.
	// A zero layoffState means "no cut": its apply field is false, so active() is
	// false and no weight is touched.
	prescription, err := s.prescribe(ctx, day.ProgramID, day.ID, person.userID, layoffState{})
	if err != nil {
		return false, err
	}
	if len(prescription) == 0 {
		return false, nil
	}

	wentWell := person.persona.SessionGoesWell(rng)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	session, err := qtx.CreateSession(ctx, store.CreateSessionParams{
		ProgramDayID: day.ID,
		PerformedOn:  pgtype.Date{Time: on, Valid: true},
		UserID:       person.userID,
	})
	if err != nil {
		return false, err
	}

	for _, pe := range prescription {
		total := len(pe.SetPlan)
		for _, set := range pe.SetPlan {
			reps := person.persona.SetReps(set.Reps, int(set.SetNumber), total, wentWell, rng)
			// Inserted already logged, in one statement. See
			// CreateLoggedSessionSet for why not materialise-then-update.
			//
			// completed is set the way a lifter would tick it: true when the set
			// made its target. The progression engine reads exactly that to decide
			// whether the weight moves, so a generated session that ticked a short
			// set would advance a lift it should have stalled.
			if err := qtx.CreateLoggedSessionSet(ctx, store.CreateLoggedSessionSetParams{
				SessionID:  session.ID,
				ExerciseID: pe.ExerciseID,
				SetNumber:  set.SetNumber,
				TargetReps: set.Reps,
				WeightLb:   floatToNumeric(set.WeightLb),
				ActualReps: reps,
				Completed:  reps >= set.Reps,
			}); err != nil {
				return false, err
			}
		}
	}

	if _, err := qtx.FinishSession(ctx, store.FinishSessionParams{
		ID: session.ID, UserID: person.userID,
	}); err != nil {
		return false, err
	}

	// An evening session of a plausible length. Without this the whole thing reads
	// as having taken no time — see SetSessionClock for why that is not cosmetic.
	startedAt := on.Add(time.Duration(17+rng.Intn(4))*time.Hour +
		time.Duration(rng.Intn(60))*time.Minute)
	finishedAt := startedAt.Add(time.Duration(38+rng.Intn(35)) * time.Minute)
	if err := qtx.SetSessionClock(ctx, store.SetSessionClockParams{
		ID:         session.ID,
		UserID:     person.userID,
		StartedAt:  pgtype.Timestamptz{Time: startedAt, Valid: true},
		FinishedAt: pgtype.Timestamptz{Time: finishedAt, Valid: true},
	}); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// generateRecognition has each lifter look at what the others did and respond.
//
// Reads through ListFeedSessions, which is the same query the app's own feed uses
// — so a generated lifter reacts to exactly what they would actually have seen,
// including the exclusion of their own sessions. That exclusion is why no explicit
// self-check is needed here, and the one in addSessionReaction is still the rule
// of record.
func (s *Server) generateRecognition(
	ctx context.Context, people []simulatedLifter, rng *rand.Rand,
) (reactions, comments int, err error) {
	emoji := allowedReactionList()

	for i := range people {
		person := &people[i]
		seen, err := s.q.ListFeedSessions(ctx, store.ListFeedSessionsParams{
			ViewerID: person.userID,
			Lim:      60,
			Off:      0,
		})
		if err != nil {
			return reactions, comments, err
		}

		for _, row := range seen {
			if person.persona.Reacts(rng) {
				if err := s.q.AddSessionReaction(ctx, store.AddSessionReactionParams{
					SessionID: row.ID,
					UserID:    person.userID,
					Emoji:     activity.Emoji(emoji, rng),
				}); err != nil {
					return reactions, comments, err
				}
				reactions++
			}
			if person.persona.Comments(rng) {
				body := person.persona.Comment(rng)
				// The same two rules addSessionComment applies: trimmed, non-blank,
				// within the cap. Persona.Comment cannot produce a body that fails
				// them and is tested not to, so this is a belt on a brace — but the
				// rule lives in a handler this path does not go through, so it is
				// stated rather than assumed.
				if body == "" || len([]rune(body)) > maxCommentBody {
					continue
				}
				if _, err := s.q.AddSessionComment(ctx, store.AddSessionCommentParams{
					SessionID: row.ID,
					UserID:    person.userID,
					Body:      body,
				}); err != nil {
					return reactions, comments, err
				}
				comments++
			}
		}
	}
	return reactions, comments, nil
}

// runActivity is the live loop.
//
// One lifter acts per tick rather than all of them, which is what makes it look
// like a gym instead of a cron job: activity arrives one item at a time while a
// screen is open, which is the whole reason to watch it.
func (s *Server) runActivity(
	ctx context.Context, gen int, cancel context.CancelFunc, lifters int, tick time.Duration,
) {
	// Seeded from the tick and the roster size rather than the clock, for the same
	// reproducibility reason the backfill is.
	// Seconds, not the raw Duration: a Duration is nanoseconds, so an hour is
	// 3.6e12 and simulationRNG's shift would run off the top of an int64. The tick
	// is whole seconds by construction anyway — the handler builds it from
	// TickSeconds.
	rng := simulationRNG(int64(tick/time.Second), int64(lifters))

	// Every exit from here releases the slot — see finish, which also explains why
	// it is generation-checked rather than a plain stop(). Deferred rather than
	// repeated at each return, because there are three exits below and the next
	// one added would not remember.
	defer s.activity.finish(gen, cancel)

	// Resolved once, before the ticker, rather than per tick. Per tick it would be
	// four queries a lifter every few seconds to re-derive a roster that does not
	// change — on a deployment with one small pool shared with the reporter and the
	// sweeper, that is real contention bought for nothing.
	//
	// The risk it accepts is the accounts going away underneath the loop, and the
	// one thing that removes them — teardown — stops the loop before it deletes.
	// Anything else leaves the writes below failing into the log, which is the
	// right noise for a state nothing is supposed to reach.
	people, _, err := s.ensureGeneratedLifters(ctx, lifters)
	if err != nil {
		log.Printf("activity loop: %v", err)
		return
	}
	if len(people) == 0 {
		return
	}

	t := time.NewTicker(tick)
	defer t.Stop()
	var turn int

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			person := &people[turn%len(people)]
			turn++

			// Sometimes a session, otherwise a reaction or a comment. Training is
			// the rarer event because it is the rarer event: a lifter trains a few
			// times a week and scrolls a feed far more often.
			//
			// The len check is not defensive tidiness. rng.Intn(0) PANICS, and this
			// runs in a goroutine with no recover — chi's Recoverer wraps HTTP
			// handlers, not this — so a persona whose program has no days would
			// take the whole process down rather than return a 500. The backfill
			// path cannot hit it because it ranges over the days instead of
			// indexing them.
			//
			// Falling through to recognition rather than skipping the tick: a
			// lifter with nothing to train can still react to somebody else.
			if len(person.days) > 0 && person.persona.Trains(rng) && rng.Intn(4) == 0 {
				day := person.days[rng.Intn(len(person.days))]
				if _, err := s.generateSession(ctx, person, day, s.reportToday(), rng); err != nil {
					log.Printf("activity loop session: %v", err)
					continue
				}
				s.activity.note(person.persona.DisplayName + " logged " + day.Name)
				continue
			}

			r, c, err := s.generateRecognitionOnce(ctx, person, rng)
			if err != nil {
				log.Printf("activity loop recognition: %v", err)
				continue
			}
			switch {
			case c > 0:
				s.activity.note(person.persona.DisplayName + " commented on a session")
			case r > 0:
				s.activity.note(person.persona.DisplayName + " reacted to a session")
			}
		}
	}
}

// generateRecognitionOnce responds to at most one session, for the live loop.
func (s *Server) generateRecognitionOnce(
	ctx context.Context, person *simulatedLifter, rng *rand.Rand,
) (reactions, comments int, err error) {
	seen, err := s.q.ListFeedSessions(ctx, store.ListFeedSessionsParams{
		ViewerID: person.userID, Lim: 20, Off: 0,
	})
	if err != nil || len(seen) == 0 {
		return 0, 0, err
	}
	row := seen[rng.Intn(len(seen))]

	if person.persona.Reacts(rng) {
		if err := s.q.AddSessionReaction(ctx, store.AddSessionReactionParams{
			SessionID: row.ID,
			UserID:    person.userID,
			Emoji:     activity.Emoji(allowedReactionList(), rng),
		}); err != nil {
			return 0, 0, err
		}
		reactions++
	}
	if person.persona.Comments(rng) {
		body := person.persona.Comment(rng)
		if body != "" && len([]rune(body)) <= maxCommentBody {
			if _, err := s.q.AddSessionComment(ctx, store.AddSessionCommentParams{
				SessionID: row.ID, UserID: person.userID, Body: body,
			}); err != nil {
				return reactions, 0, err
			}
			comments++
		}
	}
	return reactions, comments, nil
}

// ---- helpers ----

// weekdayValues are the seven values program_days.weekday admits, 0 = Sunday.
var weekdayValues = [7]int32{0, 1, 2, 3, 4, 5, 6}

// simulationRNG is the only place this file builds a random source.
//
// One function so the justification is written once rather than at each call site,
// and so a future caller cannot reach for a different source by accident.
// Both seeds are folded into the one int64 NewSource takes. `a` is shifted rather
// than added so the pair maps to distinct seeds — 4 lifters over 12 weeks and 12
// over 4 should not generate the same history. Every caller's values are small
// (bounded by the handlers at 1..8, 1..26 and 5..3600), so the shift cannot reach
// the sign bit.
//
// math/rand and NOT math/rand/v2. The v2 package is the better API and it broke
// the build: gosec 2.28.0 cannot read its export data when the toolchain is newer
// than its own (`internal error in importing "math/rand/v2" (function with type
// parameters cannot have a receiver)`), which failed CI on Go 1.27 while passing
// locally on 1.26. Nothing here needs v2 — a seeded source, Float64 and Intn are
// all of it — so the dependency was not worth the incompatibility.
func simulationRNG(a, b int64) *rand.Rand {
	// #nosec G404 -- math/rand is the right choice here and crypto/rand would be
	// the wrong one: this generates fake training data, and what is required of it
	// is REPRODUCIBILITY — the same request producing the same history, so a screen
	// that looks wrong can be regenerated exactly — rather than unpredictability.
	// Nothing derived from it is a secret, a token or a password.
	return rand.New(rand.NewSource(a<<32 ^ b))
}

// allowedReactionList is the reaction allowlist as a slice.
//
// Derived from allowedReactions rather than written out, so generated reactions
// cannot drift from what addSessionReaction accepts. Sorted, because ranging a map
// is randomised and a reproducible seed is worth nothing if the choice it feeds is
// not.
func allowedReactionList() []string {
	out := make([]string, 0, len(allowedReactions))
	for emoji := range allowedReactions {
		out = append(out, emoji)
	}
	slices.Sort(out)
	return out
}

// weekdayIn is the date of the given weekday (0 = Sunday) in the week containing
// `from`, counting the week as ending on `from`.
func weekdayIn(from time.Time, weekday int) time.Time {
	delta := (int(from.Weekday()) - weekday + 7) % 7
	return from.AddDate(0, 0, -delta)
}
