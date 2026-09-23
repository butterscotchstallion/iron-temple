#!/usr/bin/env bash
# Watch a PR to green CI and a clean AI review — see AGENTS.md → "Pull requests".
#
#   scripts/pr-watch.sh          # watch the open PR for the current branch
#   scripts/pr-watch.sh 58       # …or a specific one
#   scripts/pr-watch.sh --help   # all flags
#
# This file is only a launcher. The loop itself is `pr_watch.py` in the shared
# tooling repo (gitadmin/ai-pr-review) — the same repo this one already consumes
# for the review and apply scripts — so iron-temple and homelab-gitops run one
# implementation rather than two that drift.
#
# It fixes nothing and merges nothing. It blocks until the PR needs a decision,
# prints the failing job logs or the reviewer's findings, and exits with a code
# saying which:
#
#    0  green + LGTM — hand the PR over
#    3  merged while we watched — stop; report the merge, never re-push
#    4  closed unmerged — stop
#    5  no verdict is coming (paths filter, draft, renovate, deploy-titled)
#    7  timed out
#   10  CI failed — logs printed above
#   11  the reviewer left findings — printed above
#   12  the review itself errored
#
# So the loop is: run it; on 10/11 fix and push; run it again.
#
# Env:
#   AI_REVIEW_DIR   explicit path to a checkout of the shared repo (wins)
#   AI_REVIEW_REF   branch/SHA for the cached clone (default: main) — same
#                   convention as ai-pr-apply.yml
#   GITEA_HOST      where to clone the shared repo from, and which API to poll
#   plus the token vars documented in pr_watch.py's header

set -euo pipefail

GITEA_HOST="${GITEA_HOST:-https://gitea.homelab}"
AI_REVIEW_REF="${AI_REVIEW_REF:-main}"
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/ai-pr-review"

die() { printf 'pr-watch: %s\n' "$1" >&2; exit 2; }

# Resolve the shared checkout, in order of trust:
#   1. $AI_REVIEW_DIR          — an explicit local checkout (development)
#   2. <repo>/ai-review/       — the submodule, populated in CI
#   3. a cached clone          — the sandbox case: the submodule is NOT checked
#                                out there, and its .gitmodules URL is the
#                                gitea.homelab LoadBalancer, which an off-subnet
#                                client cannot reach. A clone over $GITEA_HOST
#                                (the NodePort) is the only path that works.
resolve_shared_dir() {
  if [ -n "${AI_REVIEW_DIR:-}" ]; then
    [ -r "$AI_REVIEW_DIR/pr_watch.py" ] \
      || die "no pr_watch.py under \$AI_REVIEW_DIR ($AI_REVIEW_DIR)"
    printf '%s\n' "$AI_REVIEW_DIR"
    return
  fi

  local root
  root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
  if [ -n "$root" ] && [ -r "$root/ai-review/pr_watch.py" ]; then
    printf '%s\n' "$root/ai-review"
    return
  fi

  if [ ! -d "$CACHE_DIR/.git" ]; then
    mkdir -p "$(dirname "$CACHE_DIR")"
    git clone --quiet "$GITEA_HOST/gitadmin/ai-pr-review.git" "$CACHE_DIR" \
      || die "could not clone the shared tooling from $GITEA_HOST/gitadmin/ai-pr-review.git"
  fi
  # Refresh best-effort: a stale cached copy still beats refusing to run, so an
  # unreachable Gitea falls through to what's on disk rather than failing the
  # watch before it has started.
  if git -C "$CACHE_DIR" fetch --quiet origin "$AI_REVIEW_REF" 2>/dev/null; then
    git -C "$CACHE_DIR" reset --quiet --hard FETCH_HEAD
  else
    printf 'pr-watch: could not refresh %s — using the cached copy\n' "$CACHE_DIR" >&2
  fi
  [ -r "$CACHE_DIR/pr_watch.py" ] \
    || die "the shared repo has no pr_watch.py at $AI_REVIEW_REF — is the ref right?"
  printf '%s\n' "$CACHE_DIR"
}

SHARED="$(resolve_shared_dir)"

# `pr-watch.sh --shared-dir` prints where the tooling was found and stops. That is
# how scripts/pr-reply.sh bootstraps without a second copy of the resolver above:
# finding the shared checkout is this file's whole job, so it may as well answer
# the question directly.
if [ "${1:-}" = "--shared-dir" ]; then
  printf '%s\n' "$SHARED"
  exit 0
fi

# Run from the CURRENT directory, not the shared checkout: pr_watch.py reads the
# git remote to work out which repo to poll, so a `cd` into the tooling would make
# it watch the tooling repo. Only the module path comes from $SHARED.
exec python3 "$SHARED/pr_watch.py" "$@"
