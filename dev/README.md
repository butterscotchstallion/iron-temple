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

## Apply migrations
The API server isn't built yet — `src/api` currently ships only the migrate
command. Point it at the local database and apply the schema:
```sh
export DATABASE_URL="postgres://iron_temple:iron_temple@localhost:5432/iron_temple?sslmode=disable"
cd src/api && make migrate
```

`make migrate` applies the embedded migrations and exits. It connects as the
`iron_temple` tenant role — never the superuser. (Once the server lands, a
`make run` target will apply migrations on startup and serve the API.)

## Reset
```sh
( cd dev && docker-compose down -v )   # or `docker compose`; -v drops the pgdata volume
```

Re-provisioning happens only on a fresh volume (the init hook runs once). To
re-run provisioning, `down -v` then `up` again.

## Notes
- **Compose v1 vs v2:** this file works with both. `docker compose` (the v2
  plugin) supports `--wait`; the older standalone `docker-compose` (v1) does not.
  If `docker compose <flag>` errors with "unknown shorthand flag", the v2 plugin
  isn't installed — drop it in with:
  ```sh
  mkdir -p ~/.docker/cli-plugins
  curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 \
    -o ~/.docker/cli-plugins/docker-compose && chmod +x ~/.docker/cli-plugins/docker-compose
  ```
  — or just use the `docker-compose` v1 command shown above.
- Passwords here (`postgres` / `iron_temple`) are local-dev only. The cluster
  uses a Kubernetes Secret; never reuse these there.
- `postgres:17-alpine` matches the Testcontainers image in the API tests.
