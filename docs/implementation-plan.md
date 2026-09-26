# Iron Temple — Implementation Plan

A fitness tracking web app for logging weight-lifting sessions over time.
Derived from [`design.md`](./design.md), with decisions locked in below.

## Locked decisions

| Topic | Decision |
|---|---|
| Folder layout | `src/api` (Go) and `src/ui` (Svelte). The design doc's "api folder / ui folder" wording refers to these. |
| Weight units | **Pounds only.** No unit column, no conversion. |
| Program scope | **Seven seeded programs.** ~~StrongLifts 5×5 + variants: Advanced 3×5, and StrongLifts 5×5 Lite. All linear.~~ The StrongLifts family — 5×5, Lite, Mini, Intermediate, and Lite (Dumbbell Press) — plus Advanced 3×5 and **Madcow 5×5**. Madcow is the exception to "all linear": it ramps within a session and progresses weekly, which is why it has its own engine (`internal/progression/madcow.go`). |
| Users | **Per-user sessions.** ~~Single implicit user.~~ A `users` table, with `sessions.user_id` scoping every performance. Programs, days and exercises stay shared — the prescription is the same for everyone; only what you lifted is yours. |
| Auth | **Session cookie.** ~~None, per design doc.~~ Password login (PBKDF2-SHA256, PHC-encoded so the algorithm can be upgraded per-user on login), an opaque `it_session` cookie stored as a SHA-256 digest, and an optional 60-day "remember me" whose expiry slides forward as it is used. Registration is first-user-only: the install is reachable from the internet, so an open signup form is an open door. |
| Database | **Runtime:** existing PostgreSQL in the cluster (via `DATABASE_URL`). **Tests:** ephemeral Postgres via Testcontainers. No local docker-compose. |
| Achievements | **Stored, and reconciled on a schedule.** A catalogue table plus a ledger of *reigns* (`held_from` / `held_until`), so "held this three times" is answerable — see migration 0032. The only kind today is the **crown** for leading a leaderboard board: one per board, all five including volume, decided over the **month**, and awarded to every lifter tied at rank 1 — except that **a board led at zero crowns nobody**, since every idle account ties at zero on three of the five boards and a crown for training nothing is not a crown. Not derived per request, because the standings are the most expensive query in the app: the hourly session sweeper diffs them and writes, and the site-wide read is one partial-index scan. Consequence to keep in mind — a crown can be up to an hour behind `/leaderboard`, which stays live. **Personal records and lifetime milestones are deliberately NOT achievements**; they are derived at read time in `internal/racked` and reported on the Racked report, and restating them here would give the install two answers about one history. A milestone's rungs come from a **per-equipment ladder** (`ladderFor`) rather than one list, since which weights count as named is a fact about the loading — the plate ladder halves to bells no rack has, so a dumbbell press was being credited at weights a lifter could not assemble. Being derived, the ladder is retroactive: tuning it changes what a past month reports, which is the trade already accepted for not storing them. **Only milestones and crowns are shareable as a card of their own** — a personal record is not, deliberately, because a tier that everything qualifies for is not a tier. |
| Follows | **Asymmetric, no approval, no self-follow.** A lifter follows whoever they train with, and it decides *delivery* rather than *visibility*: achievement notifications reach the lifter who earned one plus anybody following them, and nothing became unreadable — the roster still lists everybody and `lifters.go`'s "admission is the consent" premise is intact. Not backfilled, because none is needed: your own achievements always reach you, so a fresh install is never silent. Endpoints live under `/me/following/{lifterId}` and not under `/lifters`, which is read-only by construction. Scope is achievement notifications only — the feed stays install-wide, applause and comments keep their own relationship rules, and "somebody joined" still reaches everybody because it is how anyone learns there is somebody to follow (migration 0033). |
| Houses | **A group, not a privacy boundary.** A named group of lifters with an icon, a short **sigil** every member wears beside their name, a tagline and a description — see migration 0034. **Purely additive: nothing that was visible before is hidden, and no existing query gained a `house_id` predicate.** That is the decision worth remembering, because a House is exactly the feature that could have reversed the premise `internal/api/lifters.go` states at length — that this install has no per-lifter visibility filter because admission to it is the consent — and it deliberately does not. There is an integration test asserting a lifter in no House still reads the achievements, the leaderboard and the roster. **One House at a time**, enforced by making the lifter the primary key of `house_members` rather than by a handler rule. **The founder owns it** and is the only one who edits its identity or decides join requests; ownership is a flag on the membership row, so an owner who is not a member is unrepresentable — at the cost of one state that is resolved by *reading* rather than repairing, a House whose owner deleted their account, where the longest-standing member is treated as owner. Asking to join is a **ledger, not a queue**: a lifter may ask several Houses, and the first approval supersedes the rest. **Houses deliberately do NOT scope the leaderboard or the crowns** — `/leaderboard` and `refreshCrowns` share one `computeBoards` so the install cannot hold two answers about who is top — and the icon is **picked from a closed enum, not uploaded**, so there is no `house_icons` table beside `user_avatars`. **A House is not a
follow**: following decides who *hears* about an achievement, a House decides who is
*grouped* with whom, and neither reads the other — a House-mate is not followed by
being one, and a lifter can follow somebody in no House at all. |
| Levels | **Derived on read, never stored — the opposite call to Achievements, and for a reason rather than a taste.** A lifter's level comes from counting their qualifying sessions at request time (`db/queries/levels.sql`), with the curve in `internal/levels`. A crown is stored because deriving one costs the most expensive query in the app; a level is a grouped count over an indexed column, so there is no sweeper, no reconciler, and no window in which a written total disagrees with the sessions that explain it. That buys three things worth naming: a session **edited** after it is over cannot leave the total behind — and sessions are edited after they are over, on purpose; a session **deleted** takes its experience with it; and the award is **idempotent for free**, which matters because finishing a session is queued offline and replayed, so a write-time award would have to defend against paying twice for one workout. Generated lifters get levels without `activity.go` knowing the feature exists. **A session is worth a flat 100 XP** — not scaled by volume or sets, because the badge answers "has this person been showing up" and how strong somebody is is what the leaderboard and the crowns already say. The consequence is accepted rather than defended: a one-set session earns what a twenty-set session earns. **A session qualifies when it is over AND contains real work** — the `is_over` rule copied from `sessions.sql` (so `levels.sql` is one of the copies that file says to keep in sync), plus at least one set with `actual_reps > 0`. The work filter is what the ageing half needs: without it, opening a session and abandoning it pays out twelve hours later. It reads `actual_reps` and **not `completed`**, which means "hit its target" — qualifying on that would stop paying anybody who missed a rep. **The curve escalates**: level `L` costs `(L-1) x 100`, cumulative `50*L*(L-1)`, so level 12 is 66 sessions and level 30 is 435. **Level is deliberately NOT a field on `Lifter`**, for the reason Houses and Achievements both declined it: six queries hydrate a lifter for the wire and `users.sql` requires a column added to one roster query be added to the other. It is a site-wide `GET /levels` fetched once and consulted per name, like `/houses` and `/achievements` — and unlike those, **every account is listed**, including one that has never trained, so that "untrained" and "no such lifter" are not the same answer and the client never has to hold a copy of the starting level. **Nothing announces a level-up**: there is no live-socket frame and no celebration, which is the one thing a reader will ask about — a level is an ornament that ticks over, and the lifter who just earned one has their own badge refreshed by the session they finished. **Beside a name it is the number alone; the progress into the current level is written out in exactly one place** — the Experience section of a lifter's profile (`LevelCard`, reused by the header's hover card). Drawn for whoever is being read about rather than only for the reader, on the rule the rest of that page follows: the prose changes person, the figures do not. Nothing is guarded by doing otherwise, since XP is a qualifying-session count times a hundred and that count is a tile on the same page. **The reader's own progress is drawn once more and wordlessly**, as a neon hairline straddling the bottom edge of the header (`HeaderBar`) — yours only, since a line across the whole screen is worth that much prominence only to the person who can move it. Both surfaces scale the one ratio `percentIntoLevel` returns, so two roundings of it can never be on the screen at the same time, and neither is the curve: a ratio of two figures the server sent stays true whatever `internal/levels` charges for a level. |

### Progression rules (lb)
- All three programs are **linear** and share one progression engine: **+5 lb/session** on squat, bench, row, overhead press; **+10 lb/session** on deadlift.
- Deload **10%** after **3 consecutive failed sessions** on a lift.
- Deload **10% per full week away**, capped at **50%**, after a layoff — offered on
  the program screen when the lifter has not trained in a week, never applied
  unless they say yes. Taken off the weight last actually worked, and it does not
  stack with the stall deload above: whichever single cut is deeper wins.
- The programs differ only in set count (5×5, 3×5, or 2×5), not in progression logic.

## Database strategy

Two separate concerns:

- **Tests** use **Testcontainers** (`testcontainers-go` + its `postgres` module): each integration-test run boots a throwaway Postgres container, applies migrations, and tears it down. No shared state, no cluster dependency, isolated per run.
- **Runtime** connects to the **cluster Postgres** via a `DATABASE_URL` env var (from a Kubernetes Secret in production).

### Open item — container runtime for tests
> **STATUS (2026-08-14): resolved — no container runtime was needed in the end.** The
> suite honours `TEST_DATABASE_URL` and only falls back to Testcontainers when it is
> unset, so CI points it at a throwaway Postgres pod and the sandbox at the loopback
> server `it-testdb` manages. Neither gate uses Testcontainers; the air gap this item
> describes is still real, it just no longer blocks these tests. See
> [`api-integration-tests.md`](api-integration-tests.md). Original item below.

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
Seed data (linear and A/B split unless noted):
- **StrongLifts 5×5** — A: Squat 5×5 / Bench 5×5 / Row 5×5 · B: Squat 5×5 / OHP 5×5 / Deadlift 1×5.
- **Advanced 3×5** — the graduation fork when 5×5 stalls (same lifts, 3×5).
- **StrongLifts 5×5 Lite** — A: Squat 2×5 / Bench 2×5 / Row 2×5 · B: Squat 2×5 / OHP 2×5 / Deadlift 2×5.
- **StrongLifts 5×5 Intermediate** — A/B/**C** split. A: Squat 5×5 / Bench 5×5 / Row 5×8 ·
  B: Deadlift 5×5 / Incline Bench 5×8 / Feet-Up Bench 5×8 · C: Pause Squat 5×3 /
  Pause Bench 5×3 / Pause Deadlift 2×3. Each day also calls for assistance work, which is
  the lifter's choice and so is not seeded.
- **StrongLifts 5×5 Mini** — A: Squat 2×5 / Bench 2×5 · B: Deadlift 2×5 / OHP 2×5.
- **StrongLifts 5×5 Lite (Dumbbell Press)** — Lite with the overhead press taken to
  dumbbells, for a rack without the ceiling height for a barbell (migration 0018).
- **Madcow 5×5** — **not linear.** A ramping 5×5 on an A/B/C split that progresses week to
  week rather than session to session, so it runs on its own engine rather than the shared
  linear one (migrations 0012 and 0015).
- **Glutes & Legs (Bands and Free Weights)** — the first program from outside the StrongLifts
  family, and the first to prescribe a lift at 0 lb. A: Banded Lateral Walk 3×15 / Front Squat
  3×8 / Barbell Hip Thrust 3×8 / Bulgarian Split Squat 3×8 / Banded Hip Abduction 3×20 ·
  B: Banded Glute Bridge 3×15 / Romanian Deadlift 3×8 / Walking Lunge 3×10 / Back Extension
  3×12 / Banded Kickback 3×15. The banded and bodyweight movements carry no poundage, so they
  are seeded at 0 and held there by the zero guard in `progression.NextPlan`; which band to
  pull lives in the program's description, because the schema has nowhere else to put it
  (migration 0023).
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
