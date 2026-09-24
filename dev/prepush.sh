#!/usr/bin/env sh
# pre-push gate: run the CI checks relevant to the branch about to be pushed.
#
# This is where the expensive gates live. dev/precommit.sh keeps only the cheap
# repo-wide ones, and the reason for the split is that the gates check the WORKING
# TREE, not a commit: a stack of five commits built from one tree ran the identical
# suite five times and learned the same thing five times. The last such PR spent
# 5m26s in the hook to validate one tree state. The push is what reaches CI, so the
# push is where "don't send CI something broken" belongs.
#
# WHICH GATES, decided the same way CI decides it
#
# Relevance is computed BRANCH-vs-main, exactly like .gitea/scripts/detect-relevant-
# changes, and for the same reason: the question is "what does this branch change",
# not "what did the last commit change". A backend branch whose final commit only
# touches docs still needs the backend gates, because the backend is what is about
# to merge. See that script's header for the incident behind the rule.
#
# It deliberately ignores the refs lefthook passes on stdin. Those describe the push
# (which can be a force-push, a re-push of an unchanged branch, or several refs at
# once); the branch-vs-main diff describes the tree, which is what the gates check.
#
# Fails SAFE: anything it cannot work out — no origin/main, no merge-base, a detached
# HEAD — runs every gate rather than none. A hook that skips silently is the failure
# this whole arrangement exists to prevent.
#
# Bypass with `git push --no-verify`.
set -eu

root=$(git rev-parse --show-toplevel)

base_branch=${BASE_BRANCH:-main}
everything() {
  echo "pre-push: $1 → running every gate" >&2
  # shellcheck disable=SC2086  # deliberate word split
  exec sh "$root/scripts/preflight.sh" --strict
}

head=$(git rev-parse HEAD 2>/dev/null) || everything "no HEAD"

# origin/<base> rather than the local branch: the local one can be arbitrarily
# stale, and a stale base widens the diff, which errs toward running more gates
# rather than fewer. Absent entirely (a fresh clone that never fetched) → run all.
git rev-parse --verify --quiet "refs/remotes/origin/$base_branch" >/dev/null ||
  everything "no origin/$base_branch"

base=$(git merge-base "refs/remotes/origin/$base_branch" "$head" 2>/dev/null || true)
[ -n "$base" ] || everything "no merge-base with origin/$base_branch"

# On the base branch itself the merge-base is HEAD, which diffs nothing — the same
# degenerate case detect-relevant-changes handles. Check everything instead of
# nothing; pushing straight to main is rare enough here that the cost is noise.
[ "$base" != "$head" ] || everything "HEAD is on origin/$base_branch"

# Capture, then match. `git diff | grep -q` is a SIGPIPE race under pipefail —
# grep exits at the first match and can kill git mid-write, turning a diff that
# matched into "nothing relevant". Same note as detect-relevant-changes.
changed=$(git diff --name-only "$base" "$head")
[ -n "$changed" ] || exit 0

matches() { printf '%s\n' "$changed" | grep -qE "$1"; }

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

# Repo-wide again, cheaply. precommit.sh already ran these, but only over the
# commits it saw: a `--no-verify` commit, a rebase, or a cherry-pick can put a
# secret or an unlinted script into the range without this hook's other half ever
# having looked at it. Seconds, at the last point before it leaves the machine.
args="$args --repo"

[ -n "$args" ] || exit 0

echo "pre-push: gates for $base..$head →$args" >&2
# shellcheck disable=SC2086  # $args is a deliberate list of flags
exec sh "$root/scripts/preflight.sh" $args --strict
