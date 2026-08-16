package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"gitea.homelab/gitadmin/iron-temple/api/internal/api"
	"gitea.homelab/gitadmin/iron-temple/api/internal/racked"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// The recap reporter, against a real database.
//
// The claim is the part worth testing here: it is what makes an in-process
// ticker safe to run on more than one replica, and it is expressed entirely in
// SQL, so it cannot be exercised anywhere else. The statistics it mails are
// covered by internal/racked's own suite.

func skipWithoutDB(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test requires a Docker daemon")
	}
}

// newReportUser creates an account that no other test touches, so the recap
// tests can reason about exactly one lifter's history.
func newReportUser(t *testing.T, username string) int32 {
	t.Helper()
	var id int32
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO users (username, display_name, avatar_color, password_hash, is_admin)
		 VALUES ($1, $2, '', 'x', false) RETURNING id`,
		username, strings.ToUpper(username[:1])+username[1:],
	).Scan(&id)
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

// logSessionOn plants a finished session with real logged work on a given date,
// which is the only way to put history in a period that has already closed.
func logSessionOn(t *testing.T, userID int32, on time.Time, weight float64) {
	t.Helper()
	ctx := context.Background()

	var dayID, exerciseID int32
	if err := testPool.QueryRow(ctx, `SELECT id FROM program_days ORDER BY id LIMIT 1`).Scan(&dayID); err != nil {
		t.Fatalf("find a program day: %v", err)
	}
	if err := testPool.QueryRow(ctx, `SELECT id FROM exercises ORDER BY id LIMIT 1`).Scan(&exerciseID); err != nil {
		t.Fatalf("find an exercise: %v", err)
	}

	var sessionID int32
	err := testPool.QueryRow(ctx,
		`INSERT INTO sessions (program_day_id, performed_on, user_id, created_at, finished_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		dayID, on, userID, on.Add(9*time.Hour), on.Add(10*time.Hour),
	).Scan(&sessionID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	for i := 1; i <= 5; i++ {
		_, err := testPool.Exec(ctx,
			`INSERT INTO session_sets (session_id, exercise_id, set_number, target_reps, actual_reps, weight_lb, completed)
			 VALUES ($1, $2, $3, 5, 5, $4, true)`,
			sessionID, exerciseID, i, weight)
		if err != nil {
			t.Fatalf("create set: %v", err)
		}
	}
}

// lastMonth is the period the reporter will consider on any date.
func lastMonth() (time.Time, time.Time) {
	return racked.PreviousBounds(racked.PeriodMonth, time.Now().UTC())
}

func claimParams(userID int32, start time.Time) store.ClaimReportRunParams {
	return store.ClaimReportRunParams{
		UserID:      userID,
		PeriodKind:  string(racked.PeriodMonth),
		PeriodStart: pgtype.Date{Time: start, Valid: true},
	}
}

// Exactly one claimant wins. This is the whole reason the reporter can run
// in-process on however many replicas the deployment happens to have.
func TestReportClaimIsExactlyOnce(t *testing.T) {
	skipWithoutDB(t)
	q := store.New(testPool)
	ctx := context.Background()
	userID := newReportUser(t, "claimonce")
	start, _ := lastMonth()

	first, err := q.ClaimReportRun(ctx, claimParams(userID, start))
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if first.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", first.Attempts)
	}

	// A second replica, or the next tick, gets nothing while the first holds it.
	if _, err := q.ClaimReportRun(ctx, claimParams(userID, start)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second claim err = %v, want pgx.ErrNoRows", err)
	}

	if err := q.MarkReportRunSent(ctx, first.ID); err != nil {
		t.Fatalf("mark sent: %v", err)
	}
	// And a sent recap is never claimable again — this is what stops a lifter
	// receiving March twice.
	if _, err := q.ClaimReportRun(ctx, claimParams(userID, start)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("claim after send err = %v, want pgx.ErrNoRows", err)
	}
}

// A failed send is retried on the next tick, with the attempt counted so the
// caller can eventually give up.
func TestReportClaimRetriesAfterFailure(t *testing.T) {
	skipWithoutDB(t)
	q := store.New(testPool)
	ctx := context.Background()
	userID := newReportUser(t, "claimretry")
	start, _ := lastMonth()

	first, err := q.ClaimReportRun(ctx, claimParams(userID, start))
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := q.MarkReportRunFailed(ctx, store.MarkReportRunFailedParams{
		LastError: "relay returned 502", ID: first.ID,
	}); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	second, err := q.ClaimReportRun(ctx, claimParams(userID, start))
	if err != nil {
		t.Fatalf("reclaim after failure: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("reclaim made a new row (%d vs %d)", second.ID, first.ID)
	}
	if second.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", second.Attempts)
	}
}

// A process that dies between claiming and sending would otherwise hold the
// recap hostage forever, because the row stays 'sending' with nobody working it.
func TestReportClaimReclaimsAStaleSend(t *testing.T) {
	skipWithoutDB(t)
	q := store.New(testPool)
	ctx := context.Background()
	userID := newReportUser(t, "claimstale")
	start, _ := lastMonth()

	first, err := q.ClaimReportRun(ctx, claimParams(userID, start))
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	// Still fresh, so nobody else may take it.
	if _, err := q.ClaimReportRun(ctx, claimParams(userID, start)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a fresh claim was stealable: %v", err)
	}

	_, err = testPool.Exec(ctx,
		`UPDATE report_runs SET claimed_at = now() - INTERVAL '20 minutes' WHERE id = $1`, first.ID)
	if err != nil {
		t.Fatalf("backdate claim: %v", err)
	}

	retaken, err := q.ClaimReportRun(ctx, claimParams(userID, start))
	if err != nil {
		t.Fatalf("reclaim stale: %v", err)
	}
	if retaken.ID != first.ID {
		t.Fatalf("reclaim made a new row (%d vs %d)", retaken.ID, first.ID)
	}
}

// Recipients are the lifters who trained in the period, not every account —
// otherwise someone who signed up and never lifted gets a recap of nothing.
func TestReportRecipientsAreLiftersWhoTrained(t *testing.T) {
	skipWithoutDB(t)
	q := store.New(testPool)
	ctx := context.Background()
	trained := newReportUser(t, "trained")
	idle := newReportUser(t, "idle")
	start, end := lastMonth()
	logSessionOn(t, trained, start.AddDate(0, 0, 5), 185)

	rows, err := q.ListReportRecipients(ctx, store.ListReportRecipientsParams{
		StartOn: pgtype.Date{Time: start, Valid: true},
		EndOn:   pgtype.Date{Time: end, Valid: true},
	})
	if err != nil {
		t.Fatalf("list recipients: %v", err)
	}

	var sawTrained, sawIdle bool
	for _, r := range rows {
		sawTrained = sawTrained || r.ID == trained
		sawIdle = sawIdle || r.ID == idle
	}
	if !sawTrained {
		t.Error("the lifter who trained is not owed a recap")
	}
	if sawIdle {
		t.Error("an account with no logged work is owed a recap")
	}
}

// relaySpy stands in for the homelab mail relay.
type relaySpy struct {
	mu   sync.Mutex
	sent []map[string]any
	srv  *httptest.Server
}

func newRelaySpy(t *testing.T, status int) *relaySpy {
	t.Helper()
	spy := &relaySpy{}
	spy.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		spy.mu.Lock()
		spy.sent = append(spy.sent, payload)
		spy.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(spy.srv.Close)
	return spy
}

func (s *relaySpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

func (s *relaySpy) forUser(name string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.sent {
		if subject, _ := p["subject"].(string); strings.Contains(subject, name) {
			return p
		}
	}
	return nil
}

// waitFor polls until cond holds or the deadline passes. The reporter runs on
// its own goroutine, so there is nothing to synchronise on but the effect.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// End to end: a lifter who trained last month gets exactly one recap, and the
// next pass sends nothing because the row says it is done.
func TestReporterSendsOneRecapPerPeriod(t *testing.T) {
	skipWithoutDB(t)
	userID := newReportUser(t, "recapped")
	start, _ := lastMonth()
	logSessionOn(t, userID, start.AddDate(0, 0, 3), 225)

	spy := newRelaySpy(t, http.StatusOK)
	srv := api.NewServer(testPool, "test", "test")
	srv.SetMailer(racked.NewMailer(spy.srv.URL, "alerts@homelab.local"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// A long interval: StartRackedReporter runs its first pass immediately, and
	// that pass is what this test is about.
	srv.StartRackedReporter(ctx, time.Hour)

	waitFor(t, "the recap to be sent", func() bool { return spy.forUser("Recapped") != nil })

	payload := spy.forUser("Recapped")
	if to, _ := payload["to"].(string); to != "alerts@homelab.local" {
		t.Errorf("to = %v, want the configured recipient", payload["to"])
	}
	if html, _ := payload["html"].(string); !strings.Contains(html, "Recapped lifted") {
		t.Errorf("html does not name the lifter: %.120s", html)
	}
	if subject, _ := payload["subject"].(string); !strings.Contains(subject, "Racked:") {
		t.Errorf("subject = %v", payload["subject"])
	}

	// A second pass must be silent: the sent row is what stops a duplicate.
	before := spy.count()
	cancel()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	srv2 := api.NewServer(testPool, "test", "test")
	srv2.SetMailer(racked.NewMailer(spy.srv.URL, "alerts@homelab.local"))
	srv2.StartRackedReporter(ctx2, time.Hour)

	time.Sleep(2 * time.Second)
	if got := spy.count(); got != before {
		t.Fatalf("a second pass sent %d more recaps, want 0", got-before)
	}
}

// A relay that refuses leaves the row failed and retryable, not sent — the next
// tick tries again rather than the recap being lost.
func TestReporterRecordsARelayFailure(t *testing.T) {
	skipWithoutDB(t)
	userID := newReportUser(t, "refused")
	start, _ := lastMonth()
	logSessionOn(t, userID, start.AddDate(0, 0, 4), 135)

	spy := newRelaySpy(t, http.StatusBadGateway)
	srv := api.NewServer(testPool, "test", "test")
	srv.SetMailer(racked.NewMailer(spy.srv.URL, "alerts@homelab.local"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.StartRackedReporter(ctx, time.Hour)

	waitFor(t, "the failure to be recorded", func() bool {
		var status, lastError string
		err := testPool.QueryRow(context.Background(),
			`SELECT status, last_error FROM report_runs WHERE user_id = $1 AND period_kind = 'month'`,
			userID).Scan(&status, &lastError)
		return err == nil && status == "failed" && strings.Contains(lastError, "502")
	})
}

// The reporter does nothing at all without a mailer, which is how the setting
// that disables recaps is expressed — and why the rest of the suite, which
// constructs servers freely, never sends mail.
func TestReporterIsInertWithoutAMailer(t *testing.T) {
	skipWithoutDB(t)
	userID := newReportUser(t, "nomailer")
	start, _ := lastMonth()
	logSessionOn(t, userID, start.AddDate(0, 0, 2), 95)

	srv := api.NewServer(testPool, "test", "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.StartRackedReporter(ctx, time.Hour)
	time.Sleep(time.Second)

	var claimed int
	err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM report_runs WHERE user_id = $1`, userID).Scan(&claimed)
	if err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if claimed != 0 {
		t.Fatalf("a server with no mailer claimed %d recaps", claimed)
	}
}

// An exhausted recap must go quiet and keep the reason it stopped.
//
// 'failed' stays claimable, because that is how a retry happens at all, so the
// cap has to live in the claim. Without it every tick would re-claim the same
// broken row forever — attempts climbing without bound, and a give-up message
// overwriting the relay error an operator actually needs.
func TestReportClaimStopsAfterTheAttemptCap(t *testing.T) {
	skipWithoutDB(t)
	q := store.New(testPool)
	ctx := context.Background()
	userID := newReportUser(t, "exhausted")
	start, _ := lastMonth()

	const cap = 6
	const realError = "relay returned 502: upstream unavailable"

	var attempts int32
	for i := 0; i < cap; i++ {
		run, err := q.ClaimReportRun(ctx, claimParams(userID, start))
		if err != nil {
			t.Fatalf("claim %d of %d: %v", i+1, cap, err)
		}
		attempts = run.Attempts
		if err := q.MarkReportRunFailed(ctx, store.MarkReportRunFailedParams{
			LastError: realError, ID: run.ID,
		}); err != nil {
			t.Fatalf("mark failed: %v", err)
		}
	}
	if attempts != cap {
		t.Fatalf("last attempt counted %d, want %d", attempts, cap)
	}

	// Terminal: no further tick may take it, however many times it asks.
	for i := 0; i < 3; i++ {
		if _, err := q.ClaimReportRun(ctx, claimParams(userID, start)); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("claim past the cap err = %v, want pgx.ErrNoRows", err)
		}
	}

	// And the row still says why it stopped, rather than that it stopped.
	var status, lastError string
	var stored int32
	err := testPool.QueryRow(ctx,
		`SELECT status, attempts, last_error FROM report_runs
		  WHERE user_id = $1 AND period_kind = 'month'`, userID).Scan(&status, &stored, &lastError)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if status != "failed" || stored != cap {
		t.Fatalf("row is status %q attempts %d, want failed/%d", status, stored, cap)
	}
	if lastError != realError {
		t.Fatalf("last_error = %q, want the relay's own error preserved", lastError)
	}
}

// The cap bounds the crash-loop path too: a process that dies mid-send on every
// attempt must not reclaim its own row forever.
func TestReportClaimCapsStaleReclaims(t *testing.T) {
	skipWithoutDB(t)
	q := store.New(testPool)
	ctx := context.Background()
	userID := newReportUser(t, "crashloop")
	start, _ := lastMonth()

	for i := 0; i < 6; i++ {
		run, err := q.ClaimReportRun(ctx, claimParams(userID, start))
		if err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		// Die without ever marking the row either way.
		if _, err := testPool.Exec(ctx,
			`UPDATE report_runs SET claimed_at = now() - INTERVAL '20 minutes' WHERE id = $1`,
			run.ID); err != nil {
			t.Fatalf("backdate claim: %v", err)
		}
	}

	if _, err := q.ClaimReportRun(ctx, claimParams(userID, start)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a crash-looping recap kept reclaiming: %v", err)
	}
}
