-- Bookkeeping for the Racked recap emails. See 0008_report_runs.up.sql for why
-- the table decides whether mail goes out rather than merely recording that it
-- did, and internal/api/reporter.go for the loop that drives these.
--
-- (This block sits against the first query on purpose. sqlc folds every comment
-- ahead of a `-- name:` line into that query's doc comment, and keeps the blank
-- line between a detached header and the name line as a leading newline in the
-- generated SQL string — see the same shape on CreateSession in sessions.sql.go.
-- Keeping it contiguous keeps the generated file boring.)
--
-- ListReportRecipients returns the lifters a recap is owed for a period: those
-- who actually logged work in it. Scoping by work rather than by account is
-- what keeps the reporter from mailing an empty month to someone who signed up
-- and never trained, and it uses the same actual_reps > 0 definition of real
-- work as sessions.sql.
-- name: ListReportRecipients :many
SELECT DISTINCT u.id, u.display_name, u.username
FROM users u
JOIN sessions s ON s.user_id = u.id
JOIN session_sets ss ON ss.session_id = s.id
WHERE s.performed_on >= sqlc.arg('start_on')
  AND s.performed_on <= sqlc.arg('end_on')
  AND ss.actual_reps > 0
ORDER BY u.id;

-- ClaimReportRun takes ownership of one (user, period) recap, or reports that
-- there is nothing to take.
--
-- This is the whole of the concurrency design. The UNIQUE constraint means only
-- one INSERT can win; the guard on the DO UPDATE means a row already 'sent' —
-- or 'sending' by a replica that is still working — matches nothing and the
-- statement returns no row at all. A caller that gets pgx.ErrNoRows has learned
-- that this recap is not theirs to send, which is exactly the answer it needs.
--
-- The 15-minute clause reclaims a row orphaned by a process that died between
-- claiming and sending. It is a literal rather than a parameter because it is a
-- policy constant, not a caller's choice, and because passing an interval
-- through sqlc buys a type for no benefit; tests reach it by backdating
-- claimed_at, which is how the situation arises in the first place.
--
-- The attempts < 6 guard is what makes a beaten recap stop rather than churn.
-- 'failed' has to stay claimable — that is how a retry happens at all — so
-- without a cap here every tick would re-claim the same broken row forever,
-- incrementing attempts without bound. Capping in the claim rather than in the
-- caller also keeps the real relay error in last_error: there is no give-up
-- branch that has to overwrite it with a message about giving up. An exhausted
-- row is therefore terminal and self-describing — status 'failed', attempts 6,
-- and the error that actually stopped it.
--
-- It bounds the stale-'sending' branch too, so a process that crashes mid-send
-- on every attempt cannot loop either.
--
-- Six hourly attempts is most of a working day: long enough to ride out a relay
-- restart, short enough to stop generating traffic. To retry a recap after
-- fixing the cause, reset the row: UPDATE report_runs SET attempts = 0,
-- status = 'failed' WHERE id = ...
-- name: ClaimReportRun :one
INSERT INTO report_runs (user_id, period_kind, period_start, status, attempts)
VALUES (sqlc.arg('user_id'), sqlc.arg('period_kind'), sqlc.arg('period_start'), 'sending', 1)
ON CONFLICT (user_id, period_kind, period_start) DO UPDATE
   SET status     = 'sending',
       claimed_at = now(),
       attempts   = report_runs.attempts + 1
 WHERE report_runs.attempts < 6
   AND (report_runs.status = 'failed'
        OR (report_runs.status = 'sending'
            AND report_runs.claimed_at < now() - INTERVAL '15 minutes'))
RETURNING id, attempts;

-- MarkReportRunSent closes a recap out. Terminal: nothing reopens a sent row,
-- which is what stops a lifter getting March twice.
-- name: MarkReportRunSent :exec
UPDATE report_runs
   SET status = 'sent', sent_at = now(), last_error = ''
 WHERE id = sqlc.arg('id');

-- MarkReportRunFailed records why, and leaves the row claimable again. The
-- error is kept rather than only logged because the row is the thing an
-- operator will look at when a recap did not arrive.
-- name: MarkReportRunFailed :exec
UPDATE report_runs
   SET status = 'failed', last_error = sqlc.arg('last_error')
 WHERE id = sqlc.arg('id');
