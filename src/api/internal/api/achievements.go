package api

import (
	"context"
	"log"
	"net/http"

	"gitea.homelab/gitadmin/iron-temple/api/internal/racked"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// What a lifter has earned, and the pass that decides it.
//
// # WHY THIS IS STORED AND NOT DERIVED
//
// Everything a crown means is already computable: leaderboard.go ranks every
// lifter on five boards, and whoever is first is the holder. Deriving it per
// render was considered and is not possible at any acceptable cost — that
// computation is N lifters times eight reads, run sequentially on purpose so a
// leaderboard cannot starve the request that is logging a set. A crown appears
// beside every name on the feed, the roster, the header and the notification
// panel; putting the heaviest query in the app behind all of those is not a
// trade, it is an outage.
//
// So the standings are reconciled on the hourly sweeper and written to
// lifter_achievements, and the site-wide read becomes one partial-index scan.
//
// # AND WHY THAT IS NOT A CACHE
//
// A cache would be the same rows with a different justification, and the
// difference matters for what the table is allowed to forget. These rows are the
// only record that exists of who was top in July — the standings themselves are
// computed on demand and have no history — so a reign is data, not a memo of a
// derivation, and the reconciler is written to protect it. See refreshCrowns.
//
// The cost is staleness, bounded by the sweeper's interval. The leaderboard page
// stays live; it is the ornament beside a name that catches up.

// crownPeriod is the window the crowns are decided over.
//
// The month, which is the leaderboard's own default period and therefore the
// board a lifter sees when they follow their crown to the page it came from.
// Anything else would have the ornament and the explanation disagreeing.
//
// A constant rather than one crown per period. Five boards times three periods is
// fifteen crowns, which is not recognition — at that density every lifter on a
// household install holds something and the mark stops meaning anything.
const crownPeriod = racked.PeriodMonth

// crownRank is the place that earns a crown.
//
// First, and only first. Ties share a rank (see board() in leaderboard.go), so
// this can select several lifters on one board, and all of them are crowned —
// the standings decline to pick between equal figures and so does this.
const crownRank = 1

// getAchievements serves the catalogue with everybody currently holding each
// entry.
//
// The whole catalogue in one response, for leaderboardDTO's reason: this is the
// read every client makes in order to draw crowns beside arbitrary names, so
// answering it per lifter would be a request per row of the feed.
func (s *Server) getAchievements(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	catalogue, err := s.q.ListAchievements(ctx)
	if err != nil {
		internalError(w)
		return
	}
	holders, err := s.q.ListCurrentAchievementHolders(ctx)
	if err != nil {
		internalError(w)
		return
	}

	// Grouped by slug, because an achievement can have several holders at once
	// and the wire shape says so once rather than repeating the catalogue entry
	// per lifter.
	bySlug := make(map[string][]lifterDTO, len(catalogue))
	for _, h := range holders {
		bySlug[h.AchievementSlug] = append(bySlug[h.AchievementSlug], lifterDTO{
			ID:          h.UserID,
			Username:    h.Username,
			DisplayName: h.DisplayName,
			AvatarColor: h.AvatarColor,
			HasAvatar:   h.AvatarEtag != "",
			AvatarEtag:  h.AvatarEtag,
		})
	}

	// Every catalogue entry is listed, held or not. An achievement nobody holds
	// is an ordinary state — the install is new, or a board has no entries — and
	// a client drawing "still to win" needs the row to exist.
	items := make([]achievementHoldersDTO, 0, len(catalogue))
	for _, a := range catalogue {
		held := bySlug[a.Slug]
		if held == nil {
			held = []lifterDTO{}
		}
		items = append(items, achievementHoldersDTO{
			Achievement: achievementToDTO(a),
			Holders:     held,
		})
	}

	writeJSON(w, http.StatusOK, achievementListDTO{Items: items})
}

// getLifterAchievements serves one lifter's achievements, current and past.
//
// Under /lifters/{lifterId} rather than at the top level because the subject is a
// person — the same reason /leaderboard is top-level and this is not. There is no
// /me variant: a lifter's own achievements are the same public facts as anybody
// else's, and the profile screen already knows its own id.
func (s *Server) getLifterAchievements(w http.ResponseWriter, r *http.Request) {
	// Resolved through the shared helper so an unknown id is a 404 rather than an
	// empty list. A lifter who has earned nothing and a lifter who does not exist
	// are different answers, and the second is not a profile section to draw.
	lifter, ok := s.lifterFromPath(w, r)
	if !ok {
		return
	}

	rows, err := s.q.ListLifterAchievements(r.Context(), lifter.ID)
	if err != nil {
		internalError(w)
		return
	}

	items := make([]lifterAchievementDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, lifterAchievementDTO{
			Achievement: achievementToDTO(store.Achievement{
				Slug:        row.Slug,
				Kind:        row.Kind,
				Metric:      row.Metric,
				Label:       row.Label,
				Description: row.Description,
				SortOrder:   row.SortOrder,
			}),
			HeldNow:       row.HeldNow,
			TimesHeld:     row.TimesHeld,
			FirstHeldFrom: timestamptzToString(row.FirstHeldFrom),
			LastHeldFrom:  timestamptzToString(row.LastHeldFrom),
		})
	}

	writeJSON(w, http.StatusOK, lifterAchievementListDTO{Items: items})
}

func achievementToDTO(a store.Achievement) achievementDTO {
	dto := achievementDTO{
		Slug:        a.Slug,
		Kind:        a.Kind,
		Label:       a.Label,
		Description: a.Description,
	}
	if a.Metric != nil {
		dto.Metric = *a.Metric
	}
	return dto
}

// refreshCrowns brings the ledger into line with the standings.
//
// Run from the session sweeper beside archiveOldNotifications, and once at
// startup so a fresh deploy is not crownless for an hour. Errors are logged and
// swallowed, for that function's reason: nothing depends on this for
// correctness, and the next pass is an hour away by construction.
//
// # IT IS A DIFF, AND THAT IS THE WHOLE DESIGN
//
// The obvious implementation — close everything, then open a reign for whoever
// leads now — is wrong, and wrong in a way that would not show up for weeks. A
// reign is a continuous stretch of holding something, which is what makes
// "held this three times" a number worth printing. Restamping the current holder
// every hour would make a month on top read as seven hundred reigns, and the
// count would quietly become a measure of server uptime.
//
// So an unchanged holder is left ALONE: OpenAchievementReign conflicts against
// their open row and does nothing, and their held_from still says when they took
// it. Only somebody who is genuinely new gets a row, and only a new row is news.
//
// # NO CLAIM TABLE
//
// Idempotent by construction — a second pass over unchanged standings writes
// nothing — so two replicas racing costs a wasted UPDATE and nothing else. That
// is archiveOldNotifications' reasoning rather than the daily generator's, which
// claims because a duplicate would invent training that never happened.
func (s *Server) refreshCrowns(ctx context.Context) {
	today := s.reportToday()

	// The same computation the leaderboard page runs, deliberately shared rather
	// than reimplemented: the install must not hold two answers about who is top.
	boards, _, err := s.computeBoards(ctx, crownPeriod, today, today)
	if err != nil {
		log.Printf("crown refresh: standings: %v", err)
		return
	}

	// Which achievement each board awards. Read once rather than assumed, so the
	// mapping lives in the catalogue that the client also reads its labels from
	// and a board with no crown is simply absent here.
	catalogue, err := s.q.ListAchievements(ctx)
	if err != nil {
		log.Printf("crown refresh: catalogue: %v", err)
		return
	}
	slugFor := make(map[string]string, len(catalogue))
	for _, a := range catalogue {
		if a.Kind == "crown" && a.Metric != nil {
			slugFor[*a.Metric] = a.Slug
		}
	}

	// NOTHING IS READ BACK BEFORE WRITING, and that is worth saying out loud
	// because a diff usually needs a before-picture. This one does not: the two
	// statements below express the whole comparison in SQL. The close is "end
	// every open reign on this board except these lifters", which needs no
	// knowledge of who currently holds it, and the open is an upsert whose
	// ON CONFLICT is exactly the "already holds it" case. Reading the open reigns
	// first would be a query per pass whose answer nothing could act on.
	//
	// One transaction for the whole pass. The alternative — commit per board —
	// would let a failure halfway leave two boards reconciled and three not,
	// which is a state nothing else in the app knows how to interpret and which
	// the next pass would silently paper over.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		log.Printf("crown refresh: begin: %v", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	events := s.newLiveEvents()
	taken := 0

	for _, b := range boards {
		slug, ok := slugFor[b.Metric]
		if !ok {
			continue
		}

		leaders := make([]int32, 0, len(b.Entries))
		for _, e := range b.Entries {
			// Entries are ranked ascending, so the first rank past first ends the
			// leaders. Compared rather than assumed to be index 0, because ties
			// mean there can be several and because a board can be empty.
			if e.Rank != crownRank {
				break
			}
			leaders = append(leaders, e.Lifter.ID)
		}

		// Closing before opening is the order things happen in, not a correctness
		// requirement: the two statements touch disjoint rows, since the close
		// excludes exactly the lifters the opens are for.
		//
		// What IS worth knowing is that an empty `leaders` is meaningful rather
		// than a case to skip — it closes every open reign on the board, which is
		// what a board with no entries should mean. See the query's own note on
		// why `<> ALL` on an empty array needs no special case here.
		if _, err := qtx.CloseAchievementReignsExcept(ctx, store.CloseAchievementReignsExceptParams{
			AchievementSlug: slug,
			HolderIds:       leaders,
		}); err != nil {
			log.Printf("crown refresh: close %s: %v", slug, err)
			return
		}

		for _, id := range leaders {
			opened, err := qtx.OpenAchievementReign(ctx, store.OpenAchievementReignParams{
				UserID:          id,
				AchievementSlug: slug,
			})
			if err != nil {
				log.Printf("crown refresh: open %s: %v", slug, err)
				return
			}
			// Nothing opened means they already held it. That is the steady state
			// and it is silent — announcing it would tell the install every hour
			// about a crown that has not moved.
			if opened == 0 {
				continue
			}

			told, err := qtx.CreateCrownNotifications(ctx, store.CreateCrownNotificationsParams{
				ActorID:         id,
				AchievementSlug: slug,
			})
			if err != nil {
				log.Printf("crown refresh: announce %s: %v", slug, err)
				return
			}
			events.notify(told)
			taken++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("crown refresh: commit: %v", err)
		return
	}
	// After the commit and never deferred — see liveEvents.
	events.publish()

	// Only when something moved. An hourly no-op is the steady state and does not
	// need a line in the log to say so.
	if taken > 0 {
		log.Printf("crown refresh: %d taken", taken)
	}
}
