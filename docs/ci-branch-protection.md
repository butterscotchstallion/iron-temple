# Keeping broken code out of `main`

## The problem

Broken UI code reached a PR (e.g. it failed `pnpm check`) and could still be
merged. CI *was* catching it — the failure showed up on the branch — but nothing
**blocked the merge** on that red result.

The lesson: catching a failure and *preventing the merge* are two different
layers. We already have the first. This doc sets up the second.

## Three layers, and which one to rely on

| Layer | Where it runs | Reliable here? |
|-------|---------------|----------------|
| Author-time hooks (`lefthook`) | the committer's machine | **Partly.** The sandbox image now bakes lefthook + a warm offline pnpm store, and the launcher installs the hooks per session, so the gates genuinely run here. Still bypassable with `--no-verify`, and `go mod tidy` / `govulncheck` / `trivy` can't run air-gapped — so it catches most things early, but it is not the enforcement. |
| CI (Gitea Actions) | the runner | **Yes.** `go.yml` and `ui.yml` run lint/type-check/tests on every branch push. |
| Required status checks (branch protection) | Gitea server, at merge | **This is the enforcement.** A PR cannot merge until the required checks are green — and the server can't be bypassed by a committer lacking the toolchain. |

Do **not** try to make the pre-commit hook the gate: the environment that
produces most commits here literally cannot run it. Enforce at merge time.

## What must be true (required checks)

Branch protection on `main` requires these status-check contexts to be green
before a PR can merge:

- **`UI Check & Test`** — the gate job in `ui.yml`
- **`Go Lint & Test`** — the gate job in `go.yml`

Optionally also require: `Secret Scan`, `Trivy (dependency scan)`,
`Hadolint (Dockerfile lint)`, `Validate Deploy Manifests`.

> **Copy the exact context strings from a live PR.** Gitea renders the check name
> from the workflow/job; the strings above are what the workflows are named, but
> before you flip the toggle, open a throwaway PR (see "Verify first"), look at the
> checks list on it, and require the contexts exactly as they appear there.

## Why requiring a path-filtered check would deadlock (and how these workflows avoid it)

A required status check must be reported for **every** PR. If a workflow is
filtered with `on: push: paths:` and a PR doesn't touch those paths, the workflow
never runs, the status is never posted, and the PR blocks forever waiting on a
check that will never arrive.

`ui.yml` and `go.yml` are structured to avoid this:

1. **No `on.paths` filter** — the workflow triggers on every branch push, so its
   status context is always produced.
2. **A `changes` job** does the path filtering instead (a `git diff` of the
   branch against `main`), so the expensive build/lint/test jobs still only run
   when relevant files changed. Non-push events (schedule, manual dispatch) and
   new branches run everything. Both workflows share
   `.gitea/scripts/detect-relevant-changes`.
3. **A `result` gate job** with `if: always()` always runs and posts the required
   context. It **passes** when the real job succeeded *or* was legitimately
   skipped, and **fails** when the real job failed. This is the job you require.

Net effect: an api-only PR gets a green `UI Check & Test` (build skipped, gate
passes) without running the UI suite, and a genuinely broken UI PR gets a red
`UI Check & Test`. No deadlock, no wasted e2e runs.

### Why the diff is branch-vs-`main` and not push-vs-push

Point 3 — skipped counts as a pass — is what keeps the gate from deadlocking, and
it is safe **only** if relevance is a property of the branch. It was originally
computed from the pushed range (`github.event.before..github.sha`), and that
combination is exploitable by accident:

| push | touches | backend job | required check |
|------|---------|-------------|----------------|
| 1 | `src/api` | runs, **fails** | red |
| 2 | `src/ui` only | **skipped** (irrelevant to this push) | **green** |

After push 2 the branch is green on a head whose backend was never linted, and
that head is what merges. This is not hypothetical — it is how a gosec **G115**
finding reached `main`: run #321 on `sandbox/deload-stall-surfacing` caught it, a
UI-only follow-up commit skipped the backend job, PR #70 merged clean, and the
failure only resurfaced when the next backend PR ran gosec again (fixed in #73).

Diffing the whole branch against `main` makes the answer stable across pushes:
once a branch touches `src/api`, every later push re-runs the backend job, so the
head that merges is always a head that was checked. The cost is that a docs-only
follow-up on a backend branch re-runs the backend job — cheap next to merging
unlinted code.

On `main` itself there is no branch-vs-`main` comparison to make (the merge-base
with itself is `HEAD`), so pushes to `main` still use the pushed range.

## Enable it (operator, needs repo admin)

Gitea → the `iron-temple` repo → **Settings → Branches → Branch protection rules**
→ add/edit a rule for `main`:

1. **Enable Status Check** → tick the contexts listed above.
2. Recommended alongside it:
   - **Require pull requests** (block direct pushes to `main`).
   - **Dismiss stale approvals** / **Require the branch to be up to date** if you
     want re-runs against the latest `main`.
   - Decide whether **admins** are exempt. For the guarantee to hold against fast
     merges, do **not** exempt admins (or consciously accept the bypass).
3. Save.

This can also be set via the Gitea API
(`PATCH /repos/{owner}/{repo}/branch_protections/{name}`) if you prefer config
over clicks.

## Verify first (so an untested workflow change can't lock the repo)

The workflow restructure above was authored in the sandbox and **could not be run
there** (no runner/network). Before making the checks *required*, confirm they
report correctly:

1. Open a PR that **touches `src/ui/`** → `UI Check & Test` should run the full
   build and go green (or red if you intentionally break it).
2. Open a PR that **touches only `src/api/`** (or only docs) → `UI Check & Test`
   should still report **green** quickly (build skipped, gate passed), and
   `Go Lint & Test` should run/skip symmetrically.
3. Only once both behave correctly, enable the required checks in branch
   protection.

If a gate ever reports red for the wrong reason, fix the workflow before
requiring it — a required check that can't go green blocks all merges.
