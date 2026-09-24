#!/usr/bin/env sh
# pre-commit gate: the checks that are worth paying for on EVERY commit.
#
# That is the repo-wide set, and only that set. The build and test gates moved to
# dev/prepush.sh — see its header for why, but the short version is that these gates
# check the WORKING TREE rather than a commit, so N commits built from one tree ran
# the identical suite N times and learned the same thing N times.
#
# What stays here stays for a reason rather than because it is cheap:
#
#   • gitleaks, because the whole point is to catch a secret BEFORE it enters git
#     history. Caught at push it is already in a commit you now have to rewrite;
#     caught in CI it is already pushed, and the only honest fix is rotation.
#   • hadolint and shellcheck, because they cost under a second between them and
#     the feedback is only useful while the file is still in your head. (Do not
#     open a comment line with the word "shellcheck" — it parses as a directive.
#     This one was caught by the gate.)
#
# The gates themselves live in scripts/preflight.sh, so the hooks and a manual
# preflight can't drift apart: one definition, three callers.
#
# --strict makes missing tooling BLOCK the commit rather than skip with a notice.
# A hook that silently passes when gitleaks isn't installed looks exactly like one
# that ran and found nothing. Bypass a block with `git commit --no-verify`.
#
# Note: the gates run against the WORKING TREE, not the staged snapshot, so a
# partially-staged file is checked in full.
set -eu

root=$(git rev-parse --show-toplevel)

# No --diff-filter: a deletion is relevant too (a removed component breaks importers).
staged=$(git diff --cached --name-only)
[ -n "$staged" ] || exit 0

exec sh "$root/scripts/preflight.sh" --repo --strict
