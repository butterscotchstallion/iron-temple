#!/usr/bin/env sh
# pre-commit gate: run the CI checks that are relevant to what's staged.
#
# lefthook invokes this as its ONE pre-commit command. All of the decision-making
# lives here rather than in lefthook.yml globs, for two reasons:
#
#   • Ordering and selection are explicit and testable. `pnpm generate:api` has to
#     precede `pnpm check` (the generated hey-api client is git-ignored, so on a
#     fresh clone svelte-check fails on a missing import); expressing that as a
#     shell sequence beats relying on lefthook's command-ordering rules.
#   • A glob that quietly matches nothing turns a gate into a no-op without ever
#     saying so — the same silent-pass failure this hook exists to prevent. A regex
#     over the staged paths is greppable, and mirrors CI's own change-detector jobs.
#
# The gates themselves live in scripts/preflight.sh, so the hook and a manual
# preflight can't drift apart: one definition, two callers.
#
# --strict makes missing tooling BLOCK the commit rather than skip with a notice.
# A hook that silently passes when gosec isn't installed looks exactly like one that
# ran and found nothing. Bypass a block with `git commit --no-verify`.
#
# Note: the gates run against the WORKING TREE, not the staged snapshot, so a
# partially-staged file is checked in full.
set -eu

root=$(git rev-parse --show-toplevel)

# No --diff-filter: a deletion is relevant too (a removed component breaks importers).
staged=$(git diff --cached --name-only)
[ -n "$staged" ] || exit 0

matches() { printf '%s\n' "$staged" | grep -qE "$1"; }

args=""

# Backend. Mirrors go.yml's change detector.
if matches '^(src/api/|\.gitea/workflows/go\.yml$)'; then
  args="$args --api"
fi

# Frontend. openapi.yaml feeds the generated client, so a contract-only edit is
# exactly when the type-check is most likely to catch something. Mirrors ui.yml.
if matches '^(src/ui/|src/api/openapi\.yaml$|\.gitea/workflows/ui\.yml$)'; then
  args="$args --ui"
fi

# Repo-wide. hadolint.yml triggers on Dockerfiles; secret-scan.yml runs on EVERY
# push, so the secret scan is worth running whenever anything is staged — a secret
# caught here never enters git history, while one caught in CI is already pushed.
args="$args --repo"

# shellcheck disable=SC2086  # $args is a deliberate list of flags
exec sh "$root/scripts/preflight.sh" $args --strict
