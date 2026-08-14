# Running the API integration tests

The DB-backed backend suite — `src/api/db` (migrations, plus the parity assertions) and
`src/api/internal/api` (httpexpect against a real server on a real Postgres). Everything
else in the module is a unit test that needs no database.

## The two commands

In the devcontainer sandbox, both of these work with nothing set up first:

```sh
scripts/preflight.sh --api                    # the backend gate, integration suite included
it-testdb run -- sh dev/integration-test.sh   # just the integration suite
```

That's the whole answer for day-to-day work. The rest of this page is why it looks like
that, and what to do when it doesn't work.

## Don't hand-roll the `go test` line

`dev/integration-test.sh` is the **single definition** of the invocation, and both callers
use it: the CI job (`.gitea/workflows/go.yml`, `integration`) and the local gate
(`scripts/preflight.sh --api`). It pins `-p 1` (the two packages share one database, so
their test binaries mustn't run concurrently), `-count=1` (a cached pass must not stand in
for a real run), `TZ=UTC`, and deliberately *no* `-race` and *not* `-short`.

Typing your own `go test ./db/ ./internal/api/` re-declares those flags in a third place.
That's the drift the shared script exists to prevent — and the failure mode is nasty:
a green local run that lies about CI. If you need to run one test, pass `-run` through the
script's callee rather than reinventing the command:

```sh
it-testdb run -- sh -c 'cd src/api && TZ=UTC go test -count=1 -run TestSessionLifecycle ./internal/api/'
```

## Where the database comes from

The suite takes **`TEST_DATABASE_URL`** when it's set and falls back to Testcontainers when
it isn't (`testDB` in `internal/api/integration_test.go`, mirrored in `db/migrate_test.go`).
That one fork covers every environment:

| where | Postgres | who sets `TEST_DATABASE_URL` |
|-------|----------|------------------------------|
| sandbox devcontainer | loopback server, `initdb`'d at image build | `it-testdb run` |
| CI | throwaway pod (`.gitea/ci/postgres.yaml`) | the `integration` job |
| a laptop with Docker | Testcontainers, per-run container | nobody — the fallback |

Neither gate uses Testcontainers. CI can't: the act-runner is a *host* executor with no
container runtime. The sandbox doesn't need to: `it-testdb` is baked into the
`iron-temple-cc` image at `/usr/local/bin/it-testdb`.

### `it-testdb`

The cluster is created at **image build time**, so a broken one fails the image build rather
than someone's commit hook, and an idle session costs nothing — the server starts on first
use. `it-testdb run` takes an `flock` (the devcontainer is shared across concurrent `hl`
sessions, and they'd otherwise recreate the one database underneath each other), **drops and
recreates the database**, exports `TEST_DATABASE_URL` and `TZ=UTC`, then runs your command
holding the lock for its whole lifetime.

```sh
it-testdb run -- <cmd...>   # the primary verb: fresh database, then run <cmd>
it-testdb psql [args...]    # poke at the test database (does NOT recreate it)
it-testdb ensure            # just start the server
it-testdb stop              # stop it
```

That recreate is load-bearing, not hygiene. The suite is **not idempotent against a dirty
database** by design: `TestMain` registers the first account and
`TestRegistrationClosesAfterTheFirstAccount` asserts registration then closes itself. Run it
twice against the same database and the second run dies before any test reports:

```
register primary user: status 403
FAIL	gitea.homelab/gitadmin/iron-temple/api/internal/api	0.140s
```

That 403 is a claimed install behaving correctly, not a regression. `it-testdb run` and the
CI pod both make it structurally impossible; it's only reachable if you point
`TEST_DATABASE_URL` at a database you manage yourself.

## Parity — why you can't just start your own Postgres

`db/parity_test.go` pins what the local and CI databases must agree on — **PG 17, `UTF8`,
`en_US.utf8`** — and asserts it at run time whenever `TEST_DATABASE_URL` is set. It skips on
the Testcontainers path, which claims no parity.

So starting an ad-hoc cluster to "just run the tests" doesn't work, and fails in a way worth
recognising. A bare `initdb` in this image inherits locale `C`, which drives encoding
`SQL_ASCII`:

```
--- FAIL: TestDatabaseParity (0.01s)
    parity_test.go:86: database encoding "SQL_ASCII", want "UTF8"
    parity_test.go:89: database collation "C", want "en_US.utf8" — see the constant's comment before changing it
```

The rest of the suite may well pass against such a cluster — collation rarely changes these
queries' results — which is exactly why the assertion exists rather than being left to
chance. Use `it-testdb`; it's the cluster the pins were measured against.

(`.gitea/ci/postgres.yaml` runs `postgres:17-bookworm`, not alpine, for the same reason:
musl reports `en_US.utf8` while implementing no linguistic collation, so matching the string
from a glibc box would be false parity.)

## Expected result

38 tests, no skips, ~5 seconds once the module is built:

```
ok  	gitea.homelab/gitadmin/iron-temple/api/db            0.08s
ok  	gitea.homelab/gitadmin/iron-temple/api/internal/api  4.33s
```

A `SKIP` means `-short` leaked in, or `TEST_DATABASE_URL` is unset and the parity test has
stood itself down. Under either gate, nothing in these two packages should skip.

## When it doesn't work

- **`preflight.sh --api` reports the integration gate `unavailable`** — `it-testdb` isn't on
  PATH, so you're outside the `iron-temple-cc` image. Nothing to fix locally; CI runs the
  gate. (Under `--strict`, which the commit hook uses, that becomes a failure.)
- **`it-testdb run` hangs** — another session holds the lock. It's serialised on purpose;
  concurrent runs would recreate the database under each other.
- **Only unit tests ran** — `preflight.sh --api` runs the two integration packages *twice*,
  once under `-short` (where they self-skip) and once for real. A green `go test -short`
  line is not the integration gate.
- **`pg_ctl start failed`** — the image's cluster is at `/opt/pgtest/base`; `it-testdb`
  prints the last 20 log lines. A cluster that won't start means a bad image, since
  `it-testdb selftest` proves it at build time.

Background on the sandbox side: homelab-gitops `docs/sandbox/iron-temple-postgres-plan.md`.
