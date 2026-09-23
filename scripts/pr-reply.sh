#!/usr/bin/env bash
# Reply to an AI-review finding you are deliberately NOT acting on.
#
#   scripts/pr-watch.sh        # exits 11 and prints findings S1, S2, S3
#   # …fix S1 and S2, then say why S3 is staying as it is:
#   scripts/pr-reply.sh 229 S3 "The store is reset between tests, so the leak
#                               this finding describes cannot outlive one case."
#
# The rationale is posted threaded under the finding's own line, so it lives on
# the PR rather than only in a handover message the PR never sees.
#
#   scripts/pr-reply.sh 229 --list            # findings, and which are declined
#   scripts/pr-reply.sh 229 S3 "…" --dry-run  # print the reply, post nothing
#
# It settles nothing: the operator still reviews and merges. It never merges,
# never resolves a thread, and never edits the reviewer's own comment.
#
# Two consequences of the hidden marker it writes, both deliberate:
#   * pr-watch.sh reads it, so once every outstanding finding has a rationale the
#     watch stops (exit 13) instead of reporting the same findings forever — the
#     reviewer re-raises them on every push.
#   * It is NOT the reviewer's diff-hash marker, so the reviewer's own prune
#     leaves the rationale alone.
#
# Unlike pr-watch.sh this WRITES, so it uses the write token ($GITEA_PR_TOKEN /
# $GITEA_PR_TOKEN_FILE) — the bot credential that can open PRs and comment but
# cannot push main or merge.
#
# Args pass through; `pr-reply.sh --help` for the full set.

set -euo pipefail

die() { printf 'pr-reply: %s\n' "$1" >&2; exit 2; }

# Deliberately no copy of pr-watch.sh's resolver: ask it instead. Resolve
# symlinks first, so this still finds its sibling when put on $PATH.
SELF="$(readlink -f "$0" 2>/dev/null || echo "$0")"
HERE="$(cd "$(dirname "$SELF")" && pwd)"
[ -x "$HERE/pr-watch.sh" ] \
  || die "pr-watch.sh not found next to this script ($HERE) — it locates the shared tooling."

SHARED="$("$HERE/pr-watch.sh" --shared-dir)" || die "could not locate the shared ai-review tooling"

# From the current directory, so pr_reply.py derives the repo from this repo's
# git remote — same reason as pr-watch.sh.
exec python3 "$SHARED/pr_reply.py" "$@"
