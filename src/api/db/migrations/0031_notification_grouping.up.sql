BEGIN;

-- The panel starts folding, so the badge starts counting folds.
--
-- No column changes here, and no data changes either — this migration is one
-- index, because grouping is entirely a property of the READS. The fan-out
-- queries in notifications.sql still write a row per event, which is what keeps
-- "Bob applauded, then withdrew it, then applauded again" expressible at all.
-- What changed is that ListNotificationGroups now returns one row per
-- (kind, session_id) rather than one row per notification, and the badge has to
-- agree with it: twelve applause on one session draw one row, so they have to
-- count as one unread thing. A badge reading 12 above a single row is a badge
-- nobody can reconcile with what they are looking at.
--
-- CountUnreadNotifications therefore became a grouped count:
--
--     SELECT COUNT(*) FROM (
--         SELECT 1 FROM notifications
--         WHERE user_id = $1 AND read_at IS NULL AND archived_at IS NULL
--         GROUP BY kind, session_id
--     ) grp;
--
-- 0030's index leads on user_id alone, so it can still find the caller's unread
-- rows — but it carries neither grouping column, so every row it finds needs a
-- heap fetch to learn which group it belongs to. On the endpoint every signed-in
-- client asks for once a minute, forever.
--
-- Adding the two columns makes it index-only AND pre-sorted: the index is
-- ordered by (user_id, kind, session_id), so the groups arrive already adjacent
-- and the aggregate is a GroupAggregate over a scan rather than a hash of
-- everything unread. Replaced rather than added to, for 0030's reason — an
-- unread row nobody can reach is not something the badge counts, so it is not
-- something this index needs to hold.
--
-- session_id is nullable and that is deliberate here too: 'joined' has no
-- session, NULLs group together, and so every new account on the install folds
-- into one row. The index holds those NULLs and answers that group like any
-- other.
DROP INDEX IF EXISTS notifications_unread_idx;
CREATE INDEX notifications_unread_idx
    ON notifications (user_id, kind, session_id)
    WHERE read_at IS NULL AND archived_at IS NULL;

-- NOTHING IS ADDED FOR THE LIST, and that is worth saying out loud rather than
-- leaving as an omission.
--
-- notifications_live_idx (0030) is still the access path, but the grouped read
-- can no longer stop once it has twenty rows: to know which twenty GROUPS are
-- newest it has to fold every live row the caller has. No index can avoid that —
-- the sort key of the output (a group's newest member) is computed from the rows
-- being grouped — so the honest bound is the retention window instead. 0030
-- archives at a month, which on a household install is hundreds of rows, and
-- hundreds of rows is a scan nobody will notice.
--
-- If that ever stops being true, the answer is not an index. It is a
-- last_notified_at watermark or a materialized group table, and both are a
-- bigger decision than a migration comment should pre-empt.

COMMIT;
