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
