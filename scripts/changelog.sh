#!/usr/bin/env bash
# Print release notes for the pending release: the feat/fix (Conventional Commit)
# subjects since the last STABLE tag, one per line. Used as the Gitea Release body.
set -euo pipefail

last="$(git tag --list 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -1 || true)"
last="${last:-v0.0.0}"
if git rev-parse -q --verify "refs/tags/${last}" >/dev/null 2>&1; then
  range="${last}..HEAD"
else
  range="HEAD"
fi

notes="$(git log --format='%s (%h)' "$range" \
         | grep -E '^(feat|fix)(\([^)]*\))?!?:' \
         | sed 's/^/- /' || true)"

if [ -n "$notes" ]; then
  printf '%s\n' "$notes"
else
  echo "- (no notable changes)"
fi
