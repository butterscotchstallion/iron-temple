#!/usr/bin/env bash
# Build the app for the e2e suite with a known changelog baked in.
#
# The header's changelog panel gets its notes at BUILD time (changelogVirtualModule()
# in vite.config.ts inlines them into the bundle), so unlike every other fixture in
# this suite it cannot be mocked with page.route — by the time a test runs, the notes
# are already compiled in. Playwright's globalSetup is no help either: plugin setup,
# and with it the webServer command, runs before it.
#
# Leaving it to the checkout's git history doesn't work either. scripts/changelog.sh
# reports the feat/fix commits since the last stable tag, which is legitimately empty
# straight after a release — the panel would have nothing to show and the test would
# have nothing to assert.
#
# So: plant the same JSON the release workflow writes, build, and remove it. The
# removal is the point of the trap — vite.config.ts prefers this file over git, so a
# leftover would quietly feed these fixture notes to `pnpm dev` afterwards.
set -euo pipefail

cd "$(dirname "$0")/.."

FIXTURE="changelog.generated.json"
trap 'rm -f "$FIXTURE"' EXIT

cat > "$FIXTURE" <<'JSON'
{
  "version": "v9.9.9",
  "entries": [
    "feat(ui): show the release notes when you hover the header version (abc1234)",
    "fix(api): stop 500ing on a program with no days (def5678)"
  ]
}
JSON

pnpm build
