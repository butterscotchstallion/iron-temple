# Iron Temple — Implementation Plan

A fitness tracking web app for logging weight-lifting sessions over time.
Derived from [`design.md`](./design.md), with decisions locked in below.

## Locked decisions

| Topic | Decision |
|---|---|
| Folder layout | `src/api` (Go) and `src/ui` (Svelte). The design doc's "api folder / ui folder" wording refers to these. |
| Weight units | **Pounds only.** No unit column, no conversion. |
| Program scope | StrongLifts 5×5 + variants: Advanced 3×5, and StrongLifts 5×5 Lite. All linear. |
| Users | **Single implicit user.** No `users` table, no `user_id` foreign keys. |
| Auth | None, per design doc. |
| Database | **Runtime:** existing PostgreSQL in the cluster (via `DATABASE_URL`). **Tests:** ephemeral Postgres via Testcontainers. No local docker-compose. |

### Progression rules (lb)
- All three programs are **linear** and share one progression engine: **+5 lb/session** on squat, bench, row, overhead press; **+10 lb/session** on deadlift.
- Deload **10%** after **3 consecutive failed sessions** on a lift.
- The programs differ only in set count (5×5, 3×5, or 2×5), not in progression logic.

## Database strategy

Two separate concerns:

- **Tests** use **Testcontainers** (`testcontainers-go` + its `postgres` module): each integration-test run boots a throwaway Postgres container, applies migrations, and tears it down. No shared state, no cluster dependency, isolated per run.
- **Runtime** connects to the **cluster Postgres** via a `DATABASE_URL` env var (from a Kubernetes Secret in production).

### Open item — container runtime for tests
Testcontainers needs a Docker-compatible daemon. This sandbox currently has **none** (no `docker`/`podman`, no socket, no `DOCKER_HOST`). Tests can be written now but won't execute until one of:
1. A Docker socket is mounted into the devcontainer (`/var/run/docker.sock`), **or**
2. `DOCKER_HOST` points at a remote Docker daemon reachable from the sandbox, **or**
3. Testcontainers Cloud (`TC_CLOUD_TOKEN`).

### Tenant provisioning (cluster runtime)
The shared PG17 instance uses per-tenant isolation: one login role owning one
database, `CONNECT` revoked from `PUBLIC`. The `iron-temple` tenant is
provisioned by [`deploy/`](../deploy/) — an idempotent `psql` bootstrap run as a
Kubernetes Job. Role/DB: `iron_temple`. The app connects as that role via
`DATABASE_URL` (from the `iron-temple-db` Secret), never as the superuser.

### Open item — cluster access (deferrable)
Only needed to *run* the provisioning Job / real app against the live DB — **not**
for development or tests (Testcontainers covers those). Resolve later via
`kubectl port-forward` or connection details (prefer a scoped credential; avoid
pasting real passwords in chat).

## Phases

### Phase 0 — Tooling & scaffolding
- `go.mod` in `src/api` (Go 1.26).
- Migration tool: **golang-migrate** (plain SQL migrations).
- Data layer: **sqlc** (typed queries over `database/sql`) — no heavy ORM.
- Test infra: **testcontainers-go** + its `postgres` module for integration tests.
- Linters/format: `golangci-lint`, `go vet`, `go mod tidy`.
- Config via env (`DATABASE_URL`, CORS origin, port). Add `.env` to `.gitignore`.
- Pre-commit hooks (lefthook or pre-commit) wiring the design doc's Go + UI steps.

### Phase 1 — Database
Tables:
- `exercises` — name, etc.
- `programs` — name, description, progression type (`linear` | `madcow`).
- `program_days` — a day/variation within a program (e.g. Workout A / B).
- `program_day_exercises` — prescribed sets, reps, starting weight per exercise on a day.
- `sessions` — a dated instance of a program day that was performed.
- `session_sets` — actual reps, weight, completed flag per set.

Constraints: FKs, `NOT NULL`, check constraints (weight ≥ 0, reps > 0), timestamps.
Seed data (all linear, A/B split):
- **StrongLifts 5×5** — A: Squat 5×5 / Bench 5×5 / Row 5×5 · B: Squat 5×5 / OHP 5×5 / Deadlift 1×5.
- **Advanced 3×5** — the graduation fork when 5×5 stalls (same lifts, 3×5).
- **StrongLifts 5×5 Lite** — A: Squat 2×5 / Bench 2×5 / Row 2×5 · B: Squat 2×5 / OHP 2×5 / Deadlift 2×5.
Progression is **computed** from `session_sets` history, not stored.

### Phase 2 — OpenAPI v3 spec + red integration tests
- Author `openapi.yaml` as the single source of truth (drives both the server contract and the hey-api client).
- Write `httpexpect` integration tests that fail first against unimplemented routes.
- Each test run boots a **Testcontainers** Postgres, applies migrations, then runs the suite against it (requires a container runtime — see Database strategy).

### Phase 3 — REST API (chi)
- Implement handlers to turn the red tests green.
- CORS configured for the UI origin.
- Endpoints (draft): list programs/exercises; start a session from a program day; log/patch sets; session history; computed next-session weights.
- Served under `/api/v1`.

### Phase 4 — Front end (Svelte + Tailwind)
- Scaffold in `src/ui` (TypeScript, Svelte, Tailwind).
- Generate the typed API client with `@hey-api/openapi-ts` from `openapi.yaml`.
- Synthwave aesthetic, strong purple. Mobile-first, tuned for iPad.
- 3-minute rest timer between sets (client-side only).
- Unit tests + Playwright.

## Recommendations / defaults (override anytime)
- `golang-migrate` + `sqlc` (above).
- `.env`-based config; secrets from k8s Secret in the cluster.
- Client-side rest timer (no server state).
- Encode progression as a small service so "next session" weights are computed, not hand-entered.
