#!/usr/bin/env sh
# pre-push guard: refuse to push a branch whose upstream was deleted on the remote.
#
# When a PR merges, the forge (Gitea) deletes the source branch. A later `git push` of
# that same branch name doesn't error — it silently RE-CREATES the branch as an orphan,
# disconnected from the merged PR. This hook catches that: if the current branch has a
# configured upstream but the remote no longer has that branch, the push is blocked.
#
# It reads the upstream from git config (branch.<name>.remote/.merge) rather than
# @{upstream}, so it still fires after `git fetch --prune` has dropped the stale
# remote-tracking ref. Bypass with `git push --no-verify` if you really mean to
# re-create the branch.
#
# Note: checks the CURRENT branch (HEAD) — the common `git push` case. Pushing a
# different branch by name (`git push origin other`) isn't covered.
set -eu

branch=$(git rev-parse --abbrev-ref HEAD)

remote=$(git config --get "branch.$branch.remote" 2>/dev/null || true)
merge=$(git config --get "branch.$branch.merge" 2>/dev/null || true)

# No upstream configured, or a purely local one → a legitimate new branch, allow.
[ -n "$remote" ] && [ -n "$merge" ] || exit 0
[ "$remote" = "." ] && exit 0

# `merge` is the full ref (refs/heads/<name>); match it exactly on the remote.
if [ -n "$(git ls-remote "$remote" "$merge" 2>/dev/null)" ]; then
  exit 0
fi

echo "✖ pre-push blocked: '$branch' tracks '$remote/${merge#refs/heads/}', which no longer exists on '$remote'." >&2
echo "  It was likely deleted after its PR merged — pushing would re-create an orphan branch." >&2
echo "  Start fresh instead:  git fetch origin && git switch -c <new-branch> origin/main" >&2
echo "  Bypass once (if intended):  git push --no-verify" >&2
exit 1
