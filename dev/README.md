# dev/ — local Postgres

A Docker image + Compose file that run PostgreSQL 17 locally, provisioned **the
same way as the shared cluster instance**: a dedicated `iron_temple` login role
owning the `iron_temple` database, with `CONNECT` revoked from `PUBLIC`.

`initdb/01-tenant.sql` mirrors [`../deploy/bootstrap.sql`](../deploy/bootstrap.sql);
the only difference is how it runs (the postgres image's init hook here, a
Kubernetes Job in the cluster). So local dev, Testcontainers, and prod all share
one isolation model.

## Start it
Compose auto-discovers `docker-compose.yml` in the working directory, so run it
from `dev/` — no `-f` flag needed. The subshell keeps you back at the repo root
afterwards, so the commands below all assume repo root:
```sh
( cd dev && docker compose up -d --build --wait )
```
`--wait` blocks until the healthcheck passes, so the database is ready before you
connect. (Compose V1 users: `docker-compose up -d --build`, without `--wait`.)

First start builds the image and provisions the tenant. Data persists in the
`pgdata` volume across restarts.

## Connect the API
```sh
export DATABASE_URL="postgres://iron_temple:iron_temple@localhost:5432/iron_temple?sslmode=disable"
cd src/api && make run
```

`make run` applies migrations automatically on startup; use `make migrate` to
apply them without starting the server. The app connects as the `iron_temple`
tenant role — never the superuser.

## Reset
```sh
( cd dev && docker compose down -v )   # -v drops the pgdata volume
```

Re-provisioning happens only on a fresh volume (the init hook runs once). To
re-run provisioning, `down -v` then `up` again.

## Notes
- Passwords here (`postgres` / `iron_temple`) are local-dev only. The cluster
  uses a Kubernetes Secret; never reuse these there.
- `postgres:17-alpine` matches the Testcontainers image in the API tests.
