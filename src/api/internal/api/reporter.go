package api

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"gitea.homelab/gitadmin/iron-temple/api/internal/racked"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// StartRackedReporter sends the monthly and yearly recaps, on a ticker, until
// ctx is done. It is a no-op without a mailer, which is how the setting that
// disables recaps is expressed.
//
// This is in-process rather than a CronJob, and it deliberately does not wake at
// midnight on the 1st. Every tick it asks which completed periods have no
// successful report_runs row and sends those — so the database, not the clock,
// decides what is outstanding. The consequences are worth being explicit about:
//
//   - Downtime delays a recap, it does not drop it. Down across January 1st,
//     both the December and the annual recap go out on the next tick after the
//     process returns.
//   - Running more than one replica is safe. ClaimReportRun is an atomic
//     compare-and-set on a unique constraint, so exactly one replica sends and
//     the rest do nothing. This is the one piece of state in the API that is
//     not per-replica, unlike the login rate limiter.
//   - The failure mode it cannot survive is the API being down for the whole of
//     racked.CatchUpWindow. That leaves no row rather than a wrong one, which
//     at least shows up as an absence.
//
// The first pass runs immediately rather than one tick in, so a restart catches
// up at once instead of an hour later.
func (s *Server) StartRackedReporter(ctx context.Context, every time.Duration) {
	if s.mailer == nil {
		log.Println("racked reporter disabled (no mailer configured)")
		return
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		s.sendDueReports(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.sendDueReports(ctx)
			}
		}
	}()
}

// sendDueReports runs one pass over every period a recap is owed for.
func (s *Server) sendDueReports(ctx context.Context) {
	now := time.Now().In(s.reportLocation())
	for _, due := range racked.DuePeriods(now, racked.CatchUpWindow) {
		recipients, err := s.q.ListReportRecipients(ctx, store.ListReportRecipientsParams{
			StartOn: pgtype.Date{Time: due.Start, Valid: true},
			EndOn:   pgtype.Date{Time: due.End, Valid: true},
		})
		if err != nil {
			log.Printf("racked reporter: list recipients for %s %s: %v",
				due.Kind, due.Start.Format(dateLayout), err)
			continue
		}
		for _, r := range recipients {
			if err := ctx.Err(); err != nil {
				return
			}
			s.sendReport(ctx, due, r)
		}
	}
}

// sendReport claims, builds, and sends one lifter's recap for one period.
//
// Claim before build, not after: building is the expensive part, and two
// replicas that both build and then race to insert would do the work twice and
// still need this check. A claim that returns no row means the recap is already
// sent or in flight elsewhere, and the right response is to do nothing at all.
func (s *Server) sendReport(ctx context.Context, due racked.Due, r store.ListReportRecipientsRow) {
	run, err := s.q.ClaimReportRun(ctx, store.ClaimReportRunParams{
		UserID:      r.ID,
		PeriodKind:  string(due.Kind),
		PeriodStart: pgtype.Date{Time: due.Start, Valid: true},
	})
	if err != nil {
		// No row is the normal, expected outcome, and covers three cases that
		// need no action here: another replica holds the claim, the recap has
		// already been sent, or it has exhausted its attempts. The last is
		// terminal by way of the claim's own cap rather than a branch here —
		// which is what keeps the real relay error in last_error instead of
		// overwriting it with a message about giving up. See ClaimReportRun.
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("racked reporter: claim %s for user %d: %v", due.Kind, r.ID, err)
		}
		return
	}

	report, err := s.buildRacked(ctx, r.ID, due.Kind, due.Start)
	if err != nil {
		s.failReport(ctx, run.ID, "build report: "+err.Error())
		return
	}

	name := r.DisplayName
	if name == "" {
		name = r.Username
	}
	html, err := racked.RenderEmail(name, report)
	if err != nil {
		s.failReport(ctx, run.ID, "render email: "+err.Error())
		return
	}

	if err := s.mailer.Send(ctx, racked.Subject(name, report), html); err != nil {
		log.Printf("racked reporter: send %s for user %d: %v", due.Kind, r.ID, err)
		s.failReport(ctx, run.ID, err.Error())
		return
	}

	if err := s.q.MarkReportRunSent(ctx, run.ID); err != nil {
		// The mail is already gone. Failing to record that is bad — the next
		// tick will send it again — but there is nothing to do here beyond
		// making the reason visible.
		log.Printf("racked reporter: mark sent %d: %v", run.ID, err)
	}
}

func (s *Server) failReport(ctx context.Context, id int32, reason string) {
	if err := s.q.MarkReportRunFailed(ctx, store.MarkReportRunFailedParams{
		LastError: truncateError(reason),
		ID:        id,
	}); err != nil {
		log.Printf("racked reporter: mark failed %d: %v", id, err)
	}
}

// truncateError keeps last_error readable. The column is unbounded, but a wall
// of text from a relay's HTML error page helps nobody.
func truncateError(s string) string {
	const limit = 300
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}
