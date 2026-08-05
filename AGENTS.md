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
