#!/usr/bin/env bash
# Print release notes — the releasable Conventional Commit subjects in a range,
# one "- subject (abc1234)" per line.
#
#   changelog.sh                notes for the PENDING release: last stable tag..HEAD.
#                               The Gitea Release body, and what the UI build bakes
#                               into the changelog panel.
#   changelog.sh --release TAG  notes for a release that already happened: the
#                               stable tag before TAG..TAG. The UI's dev build
#                               falls back to this, so a checkout sitting on a
#                               freshly tagged main still has notes to show
#                               instead of the empty range it gets by default.
#   changelog.sh --last-tag     print the most recent stable tag, or nothing.
#
# "Releasable" deliberately mirrors scripts/next-version.sh: feat/fix subjects,
# any type carrying a `!`, and BREAKING CHANGE body footers. The two decide the
# same question — whether a commit is worth releasing — and when they disagreed,
# a release cut solely on a breaking footer got notes reading "no notable
# changes".
set -euo pipefail

# Stable tags only, ascending. Pre-releases (v0.0.1-rc1) are not releases.
stable_tags() {
  git tag --list 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V
}

# The stable tag immediately preceding $1, empty if it is the first.
stable_before() {
  stable_tags | awk -v t="$1" '$0 == t { print prev; exit } { prev = $0 }'
}

case "${1:-}" in
  --last-tag)
    stable_tags | tail -1 || true
    exit 0
    ;;
  --release)
    tag="${2:-}"
    [ -n "$tag" ] || { echo "usage: changelog.sh --release <tag>" >&2; exit 2; }
    git rev-parse -q --verify "refs/tags/${tag}" >/dev/null 2>&1 ||
      { echo "changelog.sh: unknown tag ${tag}" >&2; exit 2; }
    prev="$(stable_before "$tag" || true)"
    # First release ever: everything up to the tag is in it.
    range="${prev:+${prev}..}${tag}"
    ;;
  "")
    last="$(stable_tags | tail -1 || true)"
    last="${last:-v0.0.0}"
    if git rev-parse -q --verify "refs/tags/${last}" >/dev/null 2>&1; then
      range="${last}..HEAD"
    else
      range="HEAD"   # no stable tag yet → consider all history
    fi
    ;;
  *)
    echo "changelog.sh: unknown argument ${1}" >&2
    exit 2
    ;;
esac

# Commits made releasable by a BREAKING CHANGE footer rather than their subject.
# Collected up front so the walk below stays a single pass in commit order.
breaking="$(git log --format='%H' --grep='^BREAKING CHANGE:' "$range" || true)"

notes=""
while IFS=$'\t' read -r hash subject short; do
  if printf '%s' "$subject" | grep -qE '^(feat|fix)(\([^)]*\))?!?:|^[a-z]+(\([^)]*\))?!:' ||
     printf '%s\n' "$breaking" | grep -qxF "$hash"; then
    notes+="- ${subject} (${short})"$'\n'
  fi
done < <(git log --format='%H%x09%s%x09%h' "$range")

if [ -n "$notes" ]; then
  printf '%s' "$notes"
else
  echo "- (no notable changes)"
fi
