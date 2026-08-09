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

# 0 none, 1 patch (fix), 2 minor (feat), 3 major (breaking).
level=0
while IFS= read -r line; do
  if printf '%s' "$line" | grep -qE '^[a-z]+(\([^)]*\))?!:' \
     || printf '%s' "$line" | grep -qE '^BREAKING CHANGE:'; then
    level=3
  elif printf '%s' "$line" | grep -qE '^feat(\([^)]*\))?:'; then
    [ "$level" -lt 2 ] && level=2
  elif printf '%s' "$line" | grep -qE '^fix(\([^)]*\))?:'; then
    [ "$level" -lt 1 ] && level=1
  fi
done < <(git log --format='%s%n%b' "$range")

[ "$level" -eq 0 ] && { echo ""; exit 0; }

IFS=. read -r maj min pat <<<"${last#v}"
case "$level" in
  3) maj=$((maj+1)); min=0; pat=0 ;;
  2) min=$((min+1)); pat=0 ;;
  1) pat=$((pat+1)) ;;
esac
echo "${maj}.${min}.${pat}"
