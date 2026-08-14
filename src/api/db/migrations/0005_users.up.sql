BEGIN;

-- Accounts. Registration is first-user-only (see the CountUsers guard in the
-- API), so this table holds one row on a typical homelab install and admits
-- more only by deliberate act.
CREATE TABLE users (
    id            SERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL DEFAULT '',
    -- Accent colour for the initials chip shown when no avatar is uploaded.
    -- Empty means "derive one from the id" — the UI owns that mapping.
    avatar_color  TEXT NOT NULL DEFAULT '',
    -- A PHC-style string that names its own algorithm and parameters, e.g.
    -- $pbkdf2-sha256$i=600000$<salt>$<hash>. Self-describing so the hashing
    -- scheme can be upgraded per-user on login without a migration. Read by
    -- exactly one query (GetUserByPassword) and never rendered in a DTO.
    password_hash TEXT NOT NULL,
    is_admin      BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Usernames are compared case-insensitively at login, so uniqueness has to be
-- case-insensitive too — otherwise "Ada" and "ada" both register and the login
-- lookup becomes ambiguous. The plain UNIQUE above stays for the exact form.
CREATE UNIQUE INDEX users_username_lower_idx ON users (lower(username));

-- At most one admin, enforced by the database.
--
-- Self-registration is first-user-only, and that guard cannot live in
-- application code alone. "Is the table empty?" is a question about rows that
-- do not exist, and COUNT(*) takes no predicate lock on them: under READ
-- COMMITTED two concurrent registrations with *different* usernames both see an
-- empty table, both insert, and both commit. Wrapping the check and the insert
-- in one transaction makes them atomic, not mutually exclusive.
-- users_username_lower_idx does not help, because the usernames differ.
--
-- The index is partial, so it constrains only admins: every row it covers has
-- is_admin = true, so uniqueness on that column admits exactly one. Ordinary
-- accounts an owner creates later are unaffected.
--
-- register() also takes an advisory lock, which is what turns the losing
-- request into a clean 403 rather than a constraint violation. This index is
-- the backstop for anything that forgets to.
CREATE UNIQUE INDEX users_single_admin_idx ON users (is_admin) WHERE is_admin;

-- Login sessions, keyed by the SHA-256 of the cookie value rather than the
-- value itself: a dump of this table yields no usable cookies. Opaque random
-- tokens (not signed/self-describing ones) mean logout and password change can
-- revoke server-side, which a stateless JWT could not.
CREATE TABLE user_sessions (
    token_hash BYTEA PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    -- true for "remember me": a 60-day persistent cookie whose expiry slides
    -- forward as it is used. false is a 24-hour browser-session cookie.
    persistent BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX user_sessions_user_idx    ON user_sessions (user_id);
CREATE INDEX user_sessions_expires_idx ON user_sessions (expires_at);

-- Avatars live in Postgres because the deployment has no object store and no
-- PVC (see deploy/) — only the tenant database. Uploads are capped at 256 KB
-- and re-encoded by the API, so these rows stay small.
CREATE TABLE user_avatars (
    user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    mime       TEXT NOT NULL,
    bytes      BYTEA NOT NULL,
    -- SHA-256 of bytes, served as the ETag so a re-upload busts client caches.
    etag       TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Training history becomes per-user. Nullable on purpose: no user exists at the
-- moment this migration runs, so NOT NULL would require inventing a
-- passwordless placeholder row. NULL means "unclaimed" — every query filters
-- user_id = $1, so such rows are invisible until the first registration adopts
-- them (AdoptOrphanSessions, run in the same transaction that creates the
-- account). Programs, days and exercises stay shared: they are the prescription,
-- not the performance.
ALTER TABLE sessions
    ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE CASCADE;

CREATE INDEX sessions_user_idx ON sessions (user_id);

COMMIT;
