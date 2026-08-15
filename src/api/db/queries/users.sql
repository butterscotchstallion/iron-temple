-- Accounts, login sessions, and avatars.
--
-- password_hash is selected by exactly one query (GetUserForLogin). Every other
-- query lists columns explicitly and omits it, so a hash cannot reach a DTO by
-- accident — the compiler stops it, because the row struct has no such field.

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

-- name: CreateUser :one
INSERT INTO users (username, display_name, password_hash, is_admin)
VALUES (sqlc.arg('username'), sqlc.arg('display_name'), sqlc.arg('password_hash'), sqlc.arg('is_admin'))
RETURNING id, username, display_name, avatar_color, is_admin, created_at, updated_at, current_program_id;

-- GetUserForLogin is the only query that reads password_hash. Username is
-- matched case-insensitively, which is why users_username_lower_idx exists.
--
-- The column list is in table order, which is what makes sqlc return the User
-- model here rather than a bespoke row struct. Keep new columns at the end.
-- name: GetUserForLogin :one
SELECT id, username, display_name, avatar_color, password_hash, is_admin, created_at, updated_at, current_program_id
FROM users
WHERE lower(username) = lower(sqlc.arg('username'));

-- name: GetUser :one
SELECT id, username, display_name, avatar_color, is_admin, created_at, updated_at, current_program_id
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
RETURNING id, username, display_name, avatar_color, is_admin, created_at, updated_at, current_program_id;

-- name: UpdateUserPassword :execrows
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
       u.current_program_id
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
