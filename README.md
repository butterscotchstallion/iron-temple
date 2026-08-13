# iron-temple

A fitness tracker for weight-lifting sessions. Go + chi API (`src/api`),
Svelte + Tailwind UI (`src/ui`), PostgreSQL.

## Development

Git hooks are managed with [lefthook](https://github.com/evilmartians/lefthook):

```sh
lefthook install
```

In the devcontainer sandbox both the binary and the install are handled for you —
the image bakes lefthook and the launcher runs `lefthook install` per session.

On commit, `lefthook.yml` runs one command, `dev/precommit.sh`, which looks at the
staged paths and runs the matching gates from `scripts/preflight.sh`:

| staged | gates |
|--------|-------|
| `src/api/**`, `.gitea/workflows/go.yml` | `go vet`, `golangci-lint`, `gosec`, `go test -short -race` |
| `src/ui/**`, `src/api/openapi.yaml` | frozen-lockfile install, `generate:api`, `svelte-check`, Vitest |
| anything | `hadolint`, `gitleaks` |

Those match what CI enforces, gate for gate and flag for flag — the point is that a
local pass should mean a CI pass. Four checks are **deliberately CI-only**, each
because it needs something an air-gapped box can't have: `go mod tidy` and
`govulncheck` (Go proxy / `vuln.go.dev`), `trivy` (vulnerability DB), and the
Playwright e2e suite (too slow for a commit hook). See the header of
`scripts/preflight.sh` for the full reasoning.

If a gate's tooling is missing the commit is **blocked**, not waved through — a hook
that silently no-ops looks exactly like one that passed. Bypass once with
`git commit --no-verify`.

Per-project setup lives in `src/api` (see its `Makefile`) and `src/ui`
(see its `README.md`).

### Preflight — check before you push

`scripts/preflight.sh` runs the same gates, so a PR doesn't come back red on
something you could catch in seconds. Selectors combine; naming none runs everything:

```sh
scripts/preflight.sh                  # everything runnable here
scripts/preflight.sh --api            # backend only
scripts/preflight.sh --ui             # frontend only
scripts/preflight.sh --repo           # Dockerfile lint + secret scan only
scripts/preflight.sh --api --repo     # combined
scripts/preflight.sh --strict         # missing tooling fails instead of skipping
```

Everything above runs **fully offline** in the sandbox: the Go module is vendored
(`src/api/vendor`), and the image bakes the Go stack plus a warm pnpm store that
`pnpm install --offline` rehydrates `node_modules` from in a few seconds.

The script exits non-zero only when a gate that actually ran failed; skipped gates
never fail it. `--strict` flips that for *missing tooling* specifically: a gate that
can't run becomes a failure instead of a skip. The pre-commit hook always passes
`--strict`; interactive runs stay lenient, so a box missing one tool can still check
the rest. ("Nothing to check here" — no `src/ui`, no `go.mod` — stays a skip either way.)
