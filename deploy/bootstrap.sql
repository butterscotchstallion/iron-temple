-- Tenant provisioning for the shared PostgreSQL 17 instance.
-- Tenant: iron-temple  ->  identifier: iron_temple
--
-- Isolation model: one dedicated login role owning exactly one database,
-- with CONNECT revoked from PUBLIC so no other tenant can reach it.
--
-- Idempotent: safe to re-run. Connect as the instance superuser to the
-- maintenance DB (postgres). The tenant password is read from the
-- APP_DB_PASSWORD environment variable (never passed on the command line).

\set ON_ERROR_STOP on
\getenv pw APP_DB_PASSWORD

-- Login role (created only if absent; password rotation is a separate ALTER ROLE)
SELECT format('CREATE ROLE iron_temple LOGIN PASSWORD %L', :'pw')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'iron_temple')\gexec

-- Database owned by the tenant role (CREATE DATABASE cannot run in a DO block,
-- hence the \gexec guard rather than IF NOT EXISTS)
SELECT 'CREATE DATABASE iron_temple OWNER iron_temple'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'iron_temple')\gexec

-- Tenant isolation: only iron_temple may connect to iron_temple
REVOKE CONNECT ON DATABASE iron_temple FROM PUBLIC;
GRANT  CONNECT ON DATABASE iron_temple TO   iron_temple;

-- Harden the public schema inside the tenant DB.
-- On PG15+ the DB owner is implicitly a member of pg_database_owner (which owns
-- public), so the GRANT below is belt-and-suspenders, but explicit is clearer.
\connect iron_temple
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT  ALL ON SCHEMA public TO   iron_temple;
