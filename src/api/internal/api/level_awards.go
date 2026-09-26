package api

import (
	"context"
	"log"

	"gitea.homelab/gitadmin/iron-temple/api/internal/levels"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// The rungs of the level ladder, and the pass that awards them.
//
// # WHY A DERIVED FIGURE GETS A STORED ACHIEVEMENT
//
// The level itself is not stored and must not be: it is a count of qualifying
// sessions (db/queries/levels.sql), so it is never stale and a deleted session takes
// its experience with it. What is stored here is something else — that a threshold
// was once PASSED.
//
// That distinction is the whole argument, and it is the one migration 0035 had to
// rewrite the AchievementKind description to make. A crown is a STANDING: it says
// who leads a board right now, and a second copy of it can contradict the boards. A
// personal record is a standing too, which is why records are deliberately not
// achievements. Reaching Level 10 is a CROSSING — an event, on a date — and no later
// edit makes it untrue, because it is not a claim about the current level at all.
//
// The consequence is accepted rather than worked around: a lifter who deletes
// sessions can sit at Level 9 holding the Level 10 rung, and both surfaces are
// right. See 0035, which says so at greater length.
//
// # NOTHING HERE EVER CLOSES A REIGN
//
// CloseAchievementReignsExcept is keyed on the achievement slug alone and has no
// notion of a permanent award, so pointing it at a level slug would quietly end
// every holder's reign. There is no schema rail against that; the rail is that this
// file does not call it, and that this paragraph says why.

// levelRung is one catalogue row that is a level's: the slug to award, and the level
// that earns it.
//
// Read from the catalogue rather than written here, so the ladder has one home. The
// alternative was a table in Go keyed by slug, which is the ladder written down
// twice and a migration away from disagreeing with itself.
type levelRung struct {
	slug  string
	level int
}

// levelLadder picks the level rungs out of the catalogue, lowest first.
//
// Rows of any other kind are skipped, and a level row with no threshold is skipped
// too — the column is nullable because every other kind leaves it NULL, so a level
// row without one is a seed bug rather than a state to guess at. Logged, because
// silently awarding nothing is how the crowns' own metric-less case would have
// failed.
func levelLadder(catalogue []store.Achievement) []levelRung {
	rungs := make([]levelRung, 0, len(catalogue))
	for _, a := range catalogue {
		if a.Kind != kindLevel {
			continue
		}
		if a.LevelThreshold == nil {
			log.Printf("level awards: catalogue row %q has no level_threshold", a.Slug)
			continue
		}
		rungs = append(rungs, levelRung{slug: a.Slug, level: int(*a.LevelThreshold)})
	}
	return rungs
}

// awardLevelRungs opens a reign for every rung this lifter has now reached, and
// tells them and their followers about the ones that are new.
//
// NO BEFORE-PICTURE IS READ, which is refreshCrowns' trick and worth repeating:
// OpenAchievementReign is :execrows over a unique partial index on open reigns, so
// "was this already held" is answered by whether the insert did anything. A rung
// they have held for a year returns 0 rows and is silently skipped; only a genuinely
// new one is news. That is also what makes this safe to call as often as it likes —
// from the sweeper and from every finished session — without paying twice for one
// crossing.
//
// Takes a *store.Queries rather than reaching for s.q, so the caller owns the
// transaction: at finish time this has to be in the same one as the finish itself.
// Collects into events and publishes nothing — the caller does that after its
// commit.
func (s *Server) awardLevelRungs(
	ctx context.Context,
	q *store.Queries,
	userID int32,
	rungs []levelRung,
	events *liveEvents,
) (awarded int, err error) {
	if len(rungs) == 0 {
		return 0, nil
	}

	sessions, err := q.CountQualifyingSessionsForLifter(ctx, userID)
	if err != nil {
		return 0, err
	}
	// The curve lives in internal/levels and is asked rather than reimplemented,
	// here as everywhere else. Comparing session counts against each rung's cost
	// would give the same answer today and be a second copy of the curve tomorrow.
	reached := levels.FromSessions(int(sessions)).Level

	for _, rung := range rungs {
		if rung.level > reached {
			continue
		}

		opened, err := q.OpenAchievementReign(ctx, store.OpenAchievementReignParams{
			UserID:          userID,
			AchievementSlug: rung.slug,
		})
		if err != nil {
			return awarded, err
		}
		if opened == 0 {
			// Already theirs. The steady state, and silent: announcing it would
			// tell a lifter every hour about a rung they passed in March.
			continue
		}

		told, err := q.CreateAchievementNotifications(ctx, store.CreateAchievementNotificationsParams{
			ActorID:         userID,
			Kind:            notificationKindLevel,
			AchievementSlug: rung.slug,
		})
		if err != nil {
			return awarded, err
		}
		if events != nil {
			events.notify(told)
		}
		awarded++
	}
	return awarded, nil
}

// refreshLevelAwards catches up every lifter on the install.
//
// Run from the session sweeper beside refreshCrowns, and it exists for one case the
// finish path cannot cover: a session that ages past the twelve-hour cutoff starts
// counting with no request having happened, so nobody was there to award the rung it
// crossed. Everything else it does is a no-op by the time it runs, because finishing
// a session already awarded it.
//
// Errors are logged and swallowed, for refreshCrowns' reason: nothing depends on
// this for correctness, and the next pass is an hour away by construction.
//
// One transaction for the whole pass, also refreshCrowns' choice. The alternative is
// a transaction per lifter, which on a household install is more round trips than
// the thing it isolates — and a pass that half-ran is not a state worth being able
// to reach.
func (s *Server) refreshLevelAwards(ctx context.Context) {
	catalogue, err := s.q.ListAchievements(ctx)
	if err != nil {
		log.Printf("level awards: catalogue: %v", err)
		return
	}
	rungs := levelLadder(catalogue)
	if len(rungs) == 0 {
		return
	}

	standings, err := s.q.ListLifterLevels(ctx)
	if err != nil {
		log.Printf("level awards: standings: %v", err)
		return
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		log.Printf("level awards: begin: %v", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	events := s.newLiveEvents()
	awarded := 0
	for _, standing := range standings {
		// Re-counted per lifter inside awardLevelRungs rather than taken from the
		// standings row above, which already has the number. One extra indexed
		// count per account buys one code path shared with the finish handler, and
		// a second way to decide who has reached what is the thing most worth not
		// having.
		got, err := s.awardLevelRungs(ctx, qtx, standing.UserID, rungs, events)
		if err != nil {
			log.Printf("level awards: lifter %d: %v", standing.UserID, err)
			return
		}
		awarded += got
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("level awards: commit: %v", err)
		return
	}
	// After the commit and never deferred — see liveEvents.
	events.publish()

	// Logged only when something happened, so an hourly no-op does not fill the log.
	if awarded > 0 {
		log.Printf("level awards: %d reached", awarded)
	}
}
