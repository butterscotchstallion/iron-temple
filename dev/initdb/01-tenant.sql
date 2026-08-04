-- Local-dev tenant provisioning. Mirrors deploy/bootstrap.sql so a local
-- Postgres matches the shared cluster instance: a dedicated iron_temple login
-- role owning the iron_temple database, with CONNECT revoked from PUBLIC.
--
-- The postgres image runs this once, on first container init, as the superuser
-- via /docker-entrypoint-initdb.d. The tenant password is read from the
-- APP_DB_PASSWORD environment variable (set in docker-compose.yml).

\set ON_ERROR_STOP on
\getenv pw APP_DB_PASSWORD

SELECT format('CREATE ROLE iron_temple LOGIN PASSWORD %L', :'pw')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'iron_temple')\gexec

SELECT 'CREATE DATABASE iron_temple OWNER iron_temple'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'iron_temple')\gexec

REVOKE CONNECT ON DATABASE iron_temple FROM PUBLIC;
GRANT  CONNECT ON DATABASE iron_temple TO   iron_temple;

\connect iron_temple
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT  ALL ON SCHEMA public TO   iron_temple;
