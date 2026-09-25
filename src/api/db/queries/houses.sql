-- Houses: the group, its members, and the requests to join one.
--
-- The reads split the way achievements.sql splits, and for the same reason.
-- ListHouses and ListHouseMemberships are asked by every signed-in client about
-- EVERY House, because a sigil has to be drawable beside any name the app
-- renders. Everything below them is asked about ONE House when its page is open.
-- The first pair is small and hot; the rest are the detail page's.
--
-- Nothing here is scoped to a caller. That is deliberate and it is the feature's
-- central decision: a House groups, it does not gate. internal/api/lifters.go
-- explains why this install has no per-lifter visibility filter, and adding one
-- here would have reversed that premise rather than extended it. Which House a
-- lifter is in is exactly as public as the name it is drawn beside.
--
-- The writes are ordinary except for two rules the SQL cannot state on its own,
-- both of which live in internal/api/houses.go: the first approval of a lifter's
-- several requests supersedes the rest, and a member leaving either transfers
-- ownership or takes the House with them.

-- ---- reading ----

-- ListHouses is the site-wide read: every House, without its description.
--
-- description is left out on purpose. It is the only unbounded column here, it
-- is read on exactly one screen, and this is the payload every client fetches on
-- load in order to draw sigils. GetHouse below is where the long version lives.
--
-- member_count rides along because the list screen shows it and the alternative
-- is a second round trip per row.
-- name: ListHouses :many
SELECT h.id,
       h.name,
       h.sigil,
       h.tagline,
       h.icon,
       h.icon_color,
       h.created_at,
       (
         SELECT COUNT(*)
         FROM house_members hm
         WHERE hm.house_id = h.id
       )::int AS member_count
FROM houses h
ORDER BY lower(h.name), h.id;

-- ListHouseMemberships is who is in what, for the whole install.
--
-- The other half of the site-wide read, and the one that actually answers "which
-- sigil goes beside this name". Returned flat rather than folded into ListHouses
-- as an array, because the client keys it into a map by lifter id and a flat list
-- is what that wants — see sigilFor in src/ui/src/lib/houses.svelte.ts.
-- name: ListHouseMemberships :many
SELECT hm.user_id,
       hm.house_id,
       hm.is_owner
FROM house_members hm
ORDER BY hm.house_id, hm.joined_at, hm.user_id;

-- GetHouse is one House in full, for its page.
-- name: GetHouse :one
SELECT h.id,
       h.name,
       h.sigil,
       h.tagline,
       h.description,
       h.icon,
       h.icon_color,
       h.created_at
FROM houses h
WHERE h.id = sqlc.arg('id')::int;

-- GetHouseOwner is the effective owner, and it is the whole of that rule in one
-- statement.
--
-- is_owner DESC puts the marked owner first when there is one. When there is not
-- — an owner deleted their account and the cascade took the membership row with
-- it before any transfer could run — the ordering falls through to the
-- longest-standing member, which is the same lifter a transfer would have
-- chosen. So an ownerless House answers this question rather than needing to be
-- repaired first, and there is no scheduled pass looking for one.
--
-- Returns no row only when the House has no members at all, which the leave path
-- makes unreachable by deleting the House instead.
-- name: GetHouseOwner :one
SELECT hm.user_id,
       hm.is_owner
FROM house_members hm
WHERE hm.house_id = sqlc.arg('house_id')::int
ORDER BY hm.is_owner DESC, hm.joined_at, hm.user_id
LIMIT 1;

-- GetHouseMembership is the caller's own membership, or no row.
--
-- One row at most, because user_id is the primary key of house_members. That is
-- "one House at a time" being read back rather than being enforced.
-- name: GetHouseMembership :one
SELECT hm.user_id,
       hm.house_id,
       hm.is_owner,
       hm.joined_at
FROM house_members hm
WHERE hm.user_id = sqlc.arg('user_id')::int;

-- ListHouseMembers is a House's members as lifters, for its page.
--
-- The same lifter columns ListLifters returns, and deliberately no more: this is
-- a social surface, and the note at the top of db/queries/users.sql explains why
-- the administrative columns stay out of one. last_trained_on rides along
-- because the member list shows it, the same way the roster does.
-- name: ListHouseMembers :many
SELECT u.id,
       u.username,
       u.display_name,
       u.avatar_color,
       COALESCE(ua.etag, '') AS avatar_etag,
       (
         SELECT MAX(s.performed_on)
         FROM sessions s
         WHERE s.user_id = u.id
           AND EXISTS (
             SELECT 1 FROM session_sets ss
             WHERE ss.session_id = s.id AND ss.actual_reps > 0
           )
       )::date AS last_trained_on,
       hm.is_owner,
       hm.joined_at
FROM house_members hm
JOIN users u ON u.id = hm.user_id
LEFT JOIN user_avatars ua ON ua.user_id = u.id
WHERE hm.house_id = sqlc.arg('house_id')::int
ORDER BY hm.is_owner DESC, hm.joined_at, u.id;

-- ListPendingHouseRequests is what an owner is shown, newest first.
--
-- Only the owner ever reads this — the handler checks before asking — which is
-- why the requester's name is joined in here rather than being fetched per row
-- by a client that would not be allowed to see the list anyway.
-- name: ListPendingHouseRequests :many
SELECT r.id,
       r.requested_at,
       u.id AS user_id,
       u.username,
       u.display_name,
       u.avatar_color,
       COALESCE(ua.etag, '') AS avatar_etag
FROM house_join_requests r
JOIN users u ON u.id = r.user_id
LEFT JOIN user_avatars ua ON ua.user_id = u.id
WHERE r.house_id = sqlc.arg('house_id')::int
  AND r.decided_at IS NULL
ORDER BY r.requested_at DESC, r.id DESC;

-- GetOpenRequest is the caller's own pending request to one House, or no row.
--
-- What the detail page's button reads: no row means "Request to join", a row
-- means "Requested" and offers to withdraw it.
-- name: GetOpenRequest :one
SELECT r.id,
       r.requested_at
FROM house_join_requests r
WHERE r.house_id = sqlc.arg('house_id')::int
  AND r.user_id = sqlc.arg('user_id')::int
  AND r.decided_at IS NULL;

-- GetJoinRequest is one request by id, for the handlers that decide it.
-- name: GetJoinRequest :one
SELECT r.id,
       r.house_id,
       r.user_id,
       r.requested_at,
       r.decided_at,
       r.outcome
FROM house_join_requests r
WHERE r.id = sqlc.arg('id')::int;

-- ---- writing ----

-- name: CreateHouse :one
INSERT INTO houses (name, sigil, tagline, description, icon, icon_color)
VALUES (sqlc.arg('name')::text,
        sqlc.arg('sigil')::text,
        sqlc.arg('tagline')::text,
        sqlc.arg('description')::text,
        sqlc.arg('icon')::text,
        sqlc.arg('icon_color')::text)
RETURNING id, name, sigil, tagline, description, icon, icon_color, created_at;

-- UpdateHouse rewrites the identity half of a House. Owner only, checked in the
-- handler — there is no caller column here to check it against.
-- name: UpdateHouse :one
UPDATE houses
SET name        = sqlc.arg('name')::text,
    sigil       = sqlc.arg('sigil')::text,
    tagline     = sqlc.arg('tagline')::text,
    description = sqlc.arg('description')::text,
    icon        = sqlc.arg('icon')::text,
    icon_color  = sqlc.arg('icon_color')::text
WHERE id = sqlc.arg('id')::int
RETURNING id, name, sigil, tagline, description, icon, icon_color, created_at;

-- name: DeleteHouse :exec
DELETE FROM houses
WHERE id = sqlc.arg('id')::int;

-- AddHouseMember joins a lifter to a House.
--
-- ON CONFLICT DO NOTHING rather than an upsert that moves them: a lifter who is
-- already in a House has to leave it first, and quietly relocating them would
-- turn the approval of a stale request into a silent transfer out of the House
-- they actually chose. :execrows so the caller can tell the difference — zero
-- means they were already somewhere, and that is a 409.
-- name: AddHouseMember :execrows
INSERT INTO house_members (user_id, house_id, is_owner)
VALUES (sqlc.arg('user_id')::int,
        sqlc.arg('house_id')::int,
        sqlc.arg('is_owner')::boolean)
ON CONFLICT (user_id) DO NOTHING;

-- name: RemoveHouseMember :execrows
DELETE FROM house_members
WHERE user_id = sqlc.arg('user_id')::int;

-- name: CountHouseMembers :one
SELECT COUNT(*)::int AS member_count
FROM house_members
WHERE house_id = sqlc.arg('house_id')::int;

-- PromoteLongestStandingMember hands a House to whoever has been in it longest.
--
-- Run after the owner's membership row is already gone, so the ordering here
-- cannot pick them again. Ties broken by user_id for the reason the leaderboard
-- breaks them that way: two rows written in the same transaction have the same
-- timestamp, and an arbitrary-but-stable answer beats a different one per call.
-- name: PromoteLongestStandingMember :execrows
UPDATE house_members
SET is_owner = true
WHERE user_id = (
    SELECT hm.user_id
    FROM house_members hm
    WHERE hm.house_id = sqlc.arg('house_id')::int
    ORDER BY hm.joined_at, hm.user_id
    LIMIT 1
);

-- name: CreateJoinRequest :one
INSERT INTO house_join_requests (house_id, user_id)
VALUES (sqlc.arg('house_id')::int, sqlc.arg('user_id')::int)
RETURNING id, house_id, user_id, requested_at;

-- DecideJoinRequest closes one request.
--
-- The decided_at IS NULL guard is what makes deciding idempotent under a double
-- tap: the second call updates nothing and :execrows reports zero, rather than
-- overwriting an approval with a decline because two taps raced.
-- name: DecideJoinRequest :execrows
UPDATE house_join_requests
SET decided_at = now(),
    decided_by = sqlc.narg('decided_by')::int,
    outcome    = sqlc.arg('outcome')::text
WHERE id = sqlc.arg('id')::int
  AND decided_at IS NULL;

-- SupersedeOtherOpenRequests closes every OTHER request a lifter has open.
--
-- Called in the same transaction as the approval that made them unanswerable. A
-- lifter may ask several Houses at once, which is the friendly behaviour; what
-- would not be friendly is leaving those requests sitting in other owners'
-- queues, where approving one would fail against a lifter who now has a House.
-- name: SupersedeOtherOpenRequests :execrows
UPDATE house_join_requests
SET decided_at = now(),
    outcome    = 'superseded'
WHERE user_id = sqlc.arg('user_id')::int
  AND id <> sqlc.arg('except_id')::int
  AND decided_at IS NULL;

-- ---- notifications ----

-- CreateHouseNotification tells one lifter about one House event.
--
-- One statement for all three kinds, because they differ only in who is told and
-- what the sentence says. The WHERE is the rule 0026 set and every fan-out in
-- this repo keeps: the actor is never a recipient, so a request tells the owner
-- and not the requester, and a decision tells the requester and not the owner.
--
-- :many returning user_id rather than :exec, matching the other fan-outs, so the
-- handler knows whose socket to wake without asking a second time.
-- name: CreateHouseNotification :many
INSERT INTO notifications (user_id, actor_id, kind, house_id)
SELECT sqlc.arg('user_id')::int,
       sqlc.arg('actor_id')::int,
       sqlc.arg('kind')::text,
       sqlc.arg('house_id')::int
WHERE sqlc.arg('user_id')::int <> sqlc.arg('actor_id')::int
RETURNING user_id;
