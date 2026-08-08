# iron-temple

A fitness tracker for weight-lifting sessions. Go + chi API (`src/api`),
Svelte + Tailwind UI (`src/ui`), PostgreSQL.

## Development

Git hooks are managed with [lefthook](https://github.com/evilmartians/lefthook):

```sh
lefthook install
```

On commit, staged changes are gated by `lefthook.yml`:

- **Go (`src/api`)** — `go mod tidy` (must be clean), `go vet`, `golangci-lint`,
  `go test -short`.
- **UI (`src/ui`)** — regenerate the hey-api client when `openapi.yaml` changes,
  `svelte-check`, Vitest unit tests.

Per-project setup lives in `src/api` (see its `Makefile`) and `src/ui`
(see its `README.md`).

### Preflight — check before you push

`scripts/preflight.sh` runs the same gates CI runs, so a PR doesn't come back
red on something you could catch in seconds:

```sh
scripts/preflight.sh          # everything runnable here
scripts/preflight.sh --api    # backend only
scripts/preflight.sh --ui     # frontend only
```

It's tuned for the **devcontainer sandbox, which has no network to any package
registry**, so the two halves behave differently there:

- **Backend gates run fully offline** — the Go module is vendored (`src/api/vendor`)
  and `go` + `golangci-lint` are baked into the sandbox image.
- **Frontend gates need `pnpm` + an installed `node_modules`**, which the sandbox
  can't fetch. When they're present (e.g. on a networked laptop) the UI gates run;
  in the sandbox they're **skipped** and CI is the backstop. Closing that gap at
  the image level is tracked in [`docs/sandbox-ui-tooling.md`](docs/sandbox-ui-tooling.md).

The script exits non-zero only when a gate that actually ran failed; skipped gates
never fail it.
