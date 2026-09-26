package api

import (
	"context"

	"gitea.homelab/gitadmin/iron-temple/api/internal/activity"
	"gitea.homelab/gitadmin/iron-temple/api/internal/auth"
)

// SendDueReportsNow runs exactly one reporter pass and returns when it is done.
//
// Test-only, and the synchronous twin of what StartRackedReporter's goroutine
// calls on a ticker. A test that wants to assert what a pass did — and
// especially one asserting a pass did NOTHING — has no signal to wait on
// otherwise, and was reduced to sleeping for longer than a pass could
// plausibly take. That is slow when it works and green when it doesn't: a pass
// delayed past the sleep looks exactly like a pass that correctly sent nothing.
//
// Callers that mean to test StartRackedReporter itself (the no-mailer guard,
// the immediate first pass) must keep using it; this bypasses that guard.
func (s *Server) SendDueReportsNow(ctx context.Context) { s.sendDueReports(ctx) }

// RefreshCrownsNow runs exactly one crown reconcile and returns when it is done.
//
// Test-only, and the synchronous twin of what StartSessionSweeper's goroutine
// calls hourly — SendDueReportsNow's reasoning applies unchanged. It matters more
// here, because most of what is worth asserting about this pass is what a SECOND
// one does not do: a reign must not be restamped, and a crown that has not moved
// must not be announced again. Sleeping cannot distinguish "did nothing, as
// intended" from "has not run yet".
func (s *Server) RefreshCrownsNow(ctx context.Context) { s.refreshCrowns(ctx) }

// EnsureGeneratedAccountNow creates or adopts one persona's account.
//
// Test-only, and unlike the two above it is not a twin of a scheduled pass — it is
// a seam onto the middle of a backfill, so a test can call it CONCURRENTLY with
// itself. That is the only way to reach the race it has to survive: the lookup and
// the insert are not one atomic step, and a backfill and a running generation loop
// walk the same fixed roster, so both can miss and both try to insert the same
// persona. Driving that through two HTTP requests would be a race on a race.
func (s *Server) EnsureGeneratedAccountNow(
	ctx context.Context, persona activity.Persona,
) (int32, bool, error) {
	return s.ensureGeneratedAccount(ctx, persona)
}

// RefreshLevelAwardsNow runs exactly one level-rung reconcile and returns when it
// is done.
//
// Test-only, RefreshCrownsNow's twin and for its reason: most of what is worth
// asserting about this pass is what a SECOND one does not do. A rung already held
// must not be re-announced, and a lifter who has dropped below one must not have it
// taken away — neither of which a sleep can tell from "has not run yet".
func (s *Server) RefreshLevelAwardsNow(ctx context.Context) { s.refreshLevelAwards(ctx) }

// SetHasher replaces the password hasher this server uses.
//
// Test-only. The _test.go suffix keeps this file out of every non-test build,
// so there is no way to reach it from cmd/server — the seam exists for the
// integration suite and cannot be turned into a production knob by accident.
//
// The suite uses it to run at a low PBKDF2 work factor. Password hashing is
// deliberately slow, and the suite creates and signs in as ~50 accounts: at
// the shipped factor that is about 80% of its runtime, none of it testing the
// hasher. internal/auth's own tests cover the shipped factor at full price.
func (s *Server) SetHasher(h auth.Hasher) { s.hasher = h }
