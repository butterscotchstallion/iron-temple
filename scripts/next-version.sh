#!/usr/bin/env bash
# Compute the next SemVer from Conventional Commits since the last STABLE tag.
# Prints "X.Y.Z" on stdout, or "" (empty) if nothing releasable landed.
set -euo pipefail

# Highest stable tag vMAJOR.MINOR.PATCH (pre-releases like -rc are ignored).
last="$(git tag --list 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -1 || true)"
last="${last:-v0.0.0}"

if git rev-parse -q --verify "refs/tags/${last}" >/dev/null 2>&1; then
  range="${last}..HEAD"
else
  range="HEAD"   # no stable tag yet → consider all history
fi

# Highest bump. Per Conventional Commits the type lives in the SUBJECT only, so
# read the type and `!` from subjects (%s). BREAKING CHANGE is a body footer —
# scan full messages (%B) for it separately. (Scanning bodies for types would
# false-trigger on squash-merge commit lists that quote other commits' subjects.)
# 0 none, 1 patch (fix/perf/revert), 2 minor (feat), 3 major (breaking).
#
# perf and revert are patch-level alongside fix, which is what semantic-release's
# default preset does and what the spec leaves open: it only fixes the meaning of
# feat and fix, and says nothing about the rest.
#
# They earn it by the same test the others pass — does a lifter get anything out
# of this release. A revert withdraws behaviour that shipped, and a perf change
# is a user-visible improvement by definition; when it is not, it is a refactor.
# A release cutting the app's entry chunk by 66% sat unreleased on main because
# every commit in it said perf, which is the case this exists to stop repeating.
#
# Everything else stays silent on purpose. refactor, test, chore, docs, build and
# ci change nothing a lifter can see, and tagging a version for them would spend
# a deploy on an identical app.
level=0
while IFS= read -r subj; do
  if printf '%s' "$subj" | grep -qE '^[a-z]+(\([^)]*\))?!:'; then
    level=3
  elif printf '%s' "$subj" | grep -qE '^feat(\([^)]*\))?:'; then
    [ "$level" -lt 2 ] && level=2
  elif printf '%s' "$subj" | grep -qE '^(fix|perf|revert)(\([^)]*\))?:'; then
    [ "$level" -lt 1 ] && level=1
  fi
done < <(git log --format='%s' "$range")
if git log --format='%B' "$range" | grep -qE '^BREAKING CHANGE:'; then
  level=3
fi

[ "$level" -eq 0 ] && { echo ""; exit 0; }

IFS=. read -r maj min pat <<<"${last#v}"
case "$level" in
  3) maj=$((maj+1)); min=0; pat=0 ;;
  2) min=$((min+1)); pat=0 ;;
  1) pat=$((pat+1)) ;;
esac
echo "${maj}.${min}.${pat}"
