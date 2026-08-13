# AGENTS

Conventions for agents (and humans) working in this repository.

## Commits

- **Use [Conventional Commits](https://www.conventionalcommits.org).** Format the
  subject as `type(scope): summary` — e.g. `feat(ui): list programs from the API`.
  Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `build`, `ci`.
  The scope is optional but encouraged (`api`, `ui`, `db`, `deploy`, `dev`, …).
- **Always include a body** describing what changed and why, separated from the
  subject by a blank line. Explain intent, not just the mechanics of the diff.
- **No line may exceed 100 characters** — subject or body. Wrap long prose.

## Generated code

- **`src/ui/src/lib/api/` is generated, not tracked.** `pnpm generate:api` builds the
  typed hey-api client from `src/api/openapi.yaml`; `src/ui/.gitignore` keeps it out of
  git. Never hand-edit it, and never commit it — change the spec instead.
- **`src/api/openapi.yaml` is the contract.** Adding an endpoint or a response field is
  a spec edit first; the client and its types follow from regeneration.
- **Every npm script that imports the client regenerates it first.** `dev`, `build`,
  `check`, `test:unit`, `test:coverage` and `test:e2e` all chain `pnpm generate:api`, so
  a stale client can't survive into a type-check. Keep that chain when adding a script —
  without it, `svelte-check` reports phantom errors ("has no exported member") against
  call sites that are correct, and a clean `git status` makes it look like main is broken.
- **Git hooks refresh it when the tree moves**, via `dev/regen-api.sh` on `post-merge`,
  `post-checkout` and `post-rewrite`. That's what keeps your editor's language server
  from linting against the previous contract after a pull.
