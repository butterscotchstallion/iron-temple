-- Accounts, login sessions, and avatars.
--
-- password_hash is selected by exactly one query IN THIS FILE (GetUserForLogin).
-- Every other query lists columns explicitly and omits it, so a hash cannot reach
-- a DTO by accident — the compiler stops it, because the row struct has no such
-- field.
--
-- There is one other reader in the directory: ListGeneratedActivityCandidates in
-- activity.sql, which uses the hash as PROOF OF ORIGIN rather than as a
-- credential. It is named here so this note stays true, and its own comment
-- carries the reasoning.

-- name: CountUsers :one
SELECT COUNT(*) AS total FROM users;

-- LockRegistration serializes the first-user check against concurrent
-- registrations. It must be taken *before* CountUsers, inside the same
-- transaction: the check asks whether any row exists, and a COUNT over rows
-- that do not exist yet takes no lock that a second transaction would block on.
-- Two racing registrations with different usernames would otherwise both see an
-- empty table and both commit.
--
-- pg_advisory_xact_lock releases at commit or rollback, so a crashed request
-- cannot wedge registration. The key is arbitrary but fixed — see
-- registrationLockKey in the api package.
-- name: LockRegistration :exec
SELECT pg_advisory_xact_lock(sqlc.arg('key')::bigint);

-- CreateUser makes an account. Both flags are explicit parameters rather than
-- defaults, because the two callers differ on both: registration creates the
-- install's single admin with a password its owner chose, and the admin area
-- creates an ordinary account with a password somebody else chose — which is
-- exactly the case must_change_password exists for.
-- name: CreateUser :one
INSERT INTO users (username, display_name, password_hash, is_admin, must_change_password)
VALUES (sqlc.arg('username'), sqlc.arg('display_name'), sqlc.arg('password_hash'), sqlc.arg('is_admin'), sqlc.arg('must_change_password'))
RETURNING id, username, display_name, avatar_color, is_admin, created_at, updated_at, current_program_id, must_change_password;

-- ListUsers is the admin roster. No password_hash, per the note at the top of
-- this file — and no gym, which the admin screen has no use for and which would
-- cost three more queries per row.
--
-- Oldest first, so the owner (the account that claimed the install) heads the
-- list and later additions append in the order they were made. id breaks ties
-- for rows created in the same tick.
-- name: ListUsers :many
SELECT id, username, display_name, avatar_color, is_admin, created_at, must_change_password
FROM users
ORDER BY created_at, id;

-- ListLifters is the social roster: every account on this install, as one
-- lifter sees another.
--
-- Deliberately not ListUsers, though the FROM clause is identical. That one
-- answers an administrative question and carries two administrative columns:
-- is_admin, and must_change_password. Neither belongs in an answer given to an
-- ordinary account. must_change_password is the sharper of the two — it means
-- "the one-time password this account was created with is still live, and still
-- known to whoever typed it", which is a fact about a credential and is the
-- owner's business alone. Sending it to every lifter on the install would leak
-- which accounts have not yet been picked up.
--
-- is_admin is milder but goes the same way: who administers the install is not
-- part of how lifters see each other, and a roster that marked one row as the
-- owner would be answering a question nobody on this screen asked.
--
-- last_trained_on is the one column this has and ListUsers does not, and it is
-- what makes the roster read as people rather than rows. It is a correlated
-- subquery rather than a LEFT JOIN with a GROUP BY because the join would fan
-- one user out per session and the grouping would have to put it back; at a
-- handful of accounts the planner runs this once per row and the cost is
-- nothing.
--
-- The EXISTS guard is the same "at least one logged rep" definition of a
-- started session that ListSessions' HAVING and SessionTotals' own EXISTS use.
-- Keeping the three in agreement is the point: a session somebody opened and
-- walked away from without logging anything is not a day they trained, and a
-- roster that dated it as one would be the app disagreeing with its own history
-- page about the same row.
--
-- NULL means this account has never logged a rep, which the API sends as an
-- absent field and the UI reads as "has not trained yet" — not as a date it
-- has to invent.
--
-- Ordered like ListUsers, and for the same reason: oldest account first, so the
-- list is stable. Sorting by last_trained_on would be the more obviously social
-- choice and is the wrong one here — it makes a roster of five people reorder
-- itself every time somebody trains, so the row a lifter reaches for moves
-- between visits to buy an ordering nobody needed at this size.
--
-- avatar_etag is joined rather than looked up per row. The avatar bytes are not
-- read — only the tag, which is what the UI needs to decide between an <img> and
-- an initials chip and to bust its cache, exactly as GetUserAvatarEtag serves
-- /me. Joining it is what keeps a roster one query instead of one per lifter;
-- LEFT, because most accounts never upload anything, and COALESCE so "no
-- avatar" arrives as an empty string rather than a NULL every caller would have
-- to branch on.
-- name: ListLifters :many
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
       )::date AS last_trained_on
FROM users u
LEFT JOIN user_avatars ua ON ua.user_id = u.id
ORDER BY u.created_at, u.id;

-- GetLifter is one row of the roster above, for the profile page.
--
-- Same column list and the same reasoning about what is left out, so the two
-- cannot disagree about what one lifter may know about another — if a column is
-- ever added to one of these, it belongs in both or in neither.
--
-- Not GetUser, which returns the administrative columns. Reusing it and simply
-- declining to copy those into the DTO would work today and is the arrangement
-- that rots: the fields would be sitting in a struct in a social handler, one
-- careless assignment away from being sent. The compiler enforcing their absence
-- is worth a near-duplicate query, which is the same trade the note at the top
-- of this file makes for password_hash.
--
-- current_program_id rides along because the profile names what this lifter is
-- currently running. It is a program id, not a prescription — shared data that
-- every account can already read through /programs.
-- name: GetLifter :one
SELECT u.id,
       u.username,
       u.display_name,
       u.avatar_color,
       COALESCE(ua.etag, '') AS avatar_etag,
       u.current_program_id,
       (
         SELECT MAX(s.performed_on)
         FROM sessions s
         WHERE s.user_id = u.id
           AND EXISTS (
             SELECT 1 FROM session_sets ss
             WHERE ss.session_id = s.id AND ss.actual_reps > 0
           )
       )::date AS last_trained_on
FROM users u
LEFT JOIN user_avatars ua ON ua.user_id = u.id
WHERE u.id = sqlc.arg('id');

-- GetUserForLogin is the only query that reads password_hash. Username is
-- matched case-insensitively, which is why users_username_lower_idx exists.
--
-- The column list is in table order, which is what makes sqlc return the User
-- model here rather than a bespoke row struct. Keep new columns at the end.
-- name: GetUserForLogin :one
SELECT id, username, display_name, avatar_color, password_hash, is_admin, created_at, updated_at, current_program_id, must_change_password
FROM users
WHERE lower(username) = lower(sqlc.arg('username'));

-- name: GetUser :one
SELECT id, username, display_name, avatar_color, is_admin, created_at, updated_at, current_program_id, must_change_password
FROM users
WHERE id = sqlc.arg('id');

-- UpdateUserProfile patches the display fields; NULL args leave a column
-- unchanged. avatar_color is COALESCEd like the rest, so clearing it back to
-- the derived default is done by sending an empty string, not null.
--
-- current_program_id is patched the same way, which means the API can set it
-- but not clear it. Nothing needs to: the column goes back to NULL when the
-- program it names is deleted, and that is the only way it should empty.
-- name: UpdateUserProfile :one
UPDATE users
SET display_name       = COALESCE(sqlc.narg('display_name'), display_name),
    avatar_color       = COALESCE(sqlc.narg('avatar_color'), avatar_color),
    current_program_id = COALESCE(sqlc.narg('current_program_id'), current_program_id),
    updated_at         = now()
WHERE id = sqlc.arg('id')
RETURNING id, username, display_name, avatar_color, is_admin, created_at, updated_at, current_program_id, must_change_password;

-- UpdateUserPassword is the user-initiated change, and it clears
-- must_change_password: an account created by the admin area reaches nothing but
-- /me and this endpoint until the flag goes, so this is the only way out of the
-- gate. Deliberately NOT the query the login re-hash uses — see
-- RehashUserPassword.
-- name: UpdateUserPassword :execrows
UPDATE users
SET password_hash        = sqlc.arg('password_hash'),
    must_change_password = false,
    updated_at           = now()
WHERE id = sqlc.arg('id');

-- RehashUserPassword upgrades a stored hash to current parameters, leaving
-- must_change_password alone.
--
-- Separate from UpdateUserPassword because it runs at a different moment for a
-- different reason: login calls it after verifying the password the account
-- already has. Sharing the query above would mean a lifter clearing the forced
-- change simply by signing in with the one-time password the admin gave them —
-- the flag would be cleared by USING that credential rather than by replacing
-- it, which is the one thing it exists to prevent.
-- name: RehashUserPassword :execrows
UPDATE users
SET password_hash = sqlc.arg('password_hash'),
    updated_at    = now()
WHERE id = sqlc.arg('id');

-- AdoptOrphanSessions hands every unowned session to a user. Run once, inside
-- the transaction that creates the first account: sessions logged before this
-- feature existed have user_id IS NULL and are invisible to every scoped query,
-- so without this the install's entire history would silently disappear.
-- name: AdoptOrphanSessions :execrows
UPDATE sessions
SET user_id = sqlc.arg('user_id')::int
WHERE user_id IS NULL;

-- ---- login sessions ----

-- name: CreateUserSession :exec
INSERT INTO user_sessions (token_hash, user_id, expires_at, persistent)
VALUES (sqlc.arg('token_hash'), sqlc.arg('user_id')::int, sqlc.arg('expires_at'), sqlc.arg('persistent'));

-- GetUserSession resolves a presented cookie to its owner, joined so that
-- authenticating a request is a single round trip. Expired rows are filtered
-- here rather than in Go: the check then cannot be forgotten by a caller, and
-- a clock skew between app and database can't extend a session.
-- name: GetUserSession :one
SELECT s.token_hash,
       s.user_id,
       s.created_at,
       s.last_seen,
       s.expires_at,
       s.persistent,
       u.username,
       u.display_name,
       u.avatar_color,
       u.is_admin,
       u.current_program_id,
       -- Joined here rather than read by a second query: the forced-change gate
       -- runs on every authenticated request, and authenticating one is meant
       -- to be a single round trip.
       u.must_change_password
FROM user_sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = sqlc.arg('token_hash')
  AND s.expires_at > now();

-- TouchUserSession slides a persistent session forward so an active user is
-- never signed out by the clock. Called only when last_seen is already stale,
-- so a busy client does not write a row per request.
-- Expiry is computed from the database's now(), not the application's clock, so
-- a skewed app pod cannot hand out sessions that outlive their intended window.
-- make_interval takes a plain integer, which keeps the parameter an int rather
-- than an interval type the driver would have to encode.
-- name: TouchUserSession :exec
UPDATE user_sessions
SET last_seen  = now(),
    expires_at = now() + make_interval(secs => sqlc.arg('ttl_seconds')::int)
WHERE token_hash = sqlc.arg('token_hash');

-- name: DeleteUserSession :exec
DELETE FROM user_sessions WHERE token_hash = sqlc.arg('token_hash');

-- DeleteUserSessionsExcept revokes every other login for a user, used on
-- password change: the point of changing a password is to lock out whoever
-- might have had the old one, which is not achieved if their cookie survives.
-- name: DeleteUserSessionsExcept :execrows
DELETE FROM user_sessions
WHERE user_id = sqlc.arg('user_id')::int
  AND token_hash <> sqlc.arg('token_hash');

-- name: DeleteExpiredUserSessions :execrows
DELETE FROM user_sessions WHERE expires_at <= now();

-- ---- avatars ----

-- name: UpsertUserAvatar :exec
INSERT INTO user_avatars (user_id, mime, bytes, etag, updated_at)
VALUES (sqlc.arg('user_id')::int, sqlc.arg('mime'), sqlc.arg('bytes'), sqlc.arg('etag'), now())
ON CONFLICT (user_id) DO UPDATE
SET mime = EXCLUDED.mime,
    bytes = EXCLUDED.bytes,
    etag = EXCLUDED.etag,
    updated_at = now();

-- name: GetUserAvatar :one
SELECT user_id, mime, bytes, etag, updated_at
FROM user_avatars
WHERE user_id = sqlc.arg('user_id')::int;

-- GetUserAvatarEtag reads just the tag, so rendering a profile does not pull
-- the image bytes through the connection to decide whether one exists.
-- name: GetUserAvatarEtag :one
SELECT etag FROM user_avatars WHERE user_id = sqlc.arg('user_id')::int;

-- name: DeleteUserAvatar :execrows
DELETE FROM user_avatars WHERE user_id = sqlc.arg('user_id')::int;
