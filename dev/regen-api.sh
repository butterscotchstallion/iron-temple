#!/usr/bin/env sh
# Refresh the git-ignored hey-api client when the OpenAPI contract has changed.
#
# src/ui/src/lib/api/ is generated from src/api/openapi.yaml and is NOT tracked
# (src/ui/.gitignore). So pulling a commit that changes the contract leaves the
# client on disk describing the OLD contract, and svelte-check fails on call
# sites that are actually correct — "has no exported member", "property does not
# exist on type". Confusing, because git status is clean and main is green.
#
# The npm scripts chain `pnpm generate:api` ahead of anything that imports the
# client, which covers every command-line entry point. This hook exists for the
# one thing they can't reach: the EDITOR. The Svelte language server reads the
# generated files off disk, so without this you get red squiggles in files you
# have not run anything against yet.
#
# Wired to post-merge, post-checkout and post-rewrite — `git pull` fires the
# first, `git pull --rebase` the third, and branch switches plus `git worktree
# add` the second.
#
# WHAT "CHANGED" MEANS HERE. Not a git diff. Asking git what changed would mean
# being right about ORIG_HEAD across three different hooks, and a path predicate
# that misses a case silently reintroduces the staleness this exists to prevent.
# Instead we hash the spec and compare against the hash recorded the last time we
# generated. That is correct however the tree moved — merge, rebase, checkout,
# worktree add, or a hand edit — and it regenerates whenever the client is simply
# missing. The same stamp and the same hash are used by the Vite dev-server plugin
# in src/ui/vite.config.ts, so the two never fight over who generated what.
#
# The stamp is written ONLY straight after a successful generation from exactly
# that content, which fixes the direction the errors fall in: it can read stale
# (a direct `pnpm generate:api` regenerates without touching it) and cost a
# redundant 250ms run, but it can never read current while the client is stale.
#
# Editing openapi-ts.config.ts changes the output without changing the spec, so
# the stamp will read as current. That is fine: the npm scripts regenerate
# unconditionally, so `pnpm dev` / `check` / `build` correct it immediately.
#
# Non-fatal by design. A post-hook that can block a checkout is worse than a stale
# client, and it is not the gate — dev/precommit.sh and CI are. This is a
# convenience that keeps the editor honest.
set -eu

root=$(git rev-parse --show-toplevel)
ui="$root/src/ui"
spec="$root/src/api/openapi.yaml"
stamp="$ui/node_modules/.cache/iron-temple/openapi-spec.sha256"

# Fresh clone: no node_modules, so openapi-ts isn't installed yet. Bail quietly —
# `pnpm install` followed by any generate-chained script covers this case.
[ -d "$ui/node_modules" ] || exit 0
[ -f "$spec" ] || exit 0

# Hash the CONTENT, via stdin so the filename stays out of the digest — that is
# what lets vite.config.ts reproduce the identical hex with node:crypto.
current=$(sha256sum < "$spec" | cut -d' ' -f1)

# Up to date only if the recorded hash matches AND a client actually exists.
if [ -d "$ui/src/lib/api" ] && [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$current" ]; then
  exit 0
fi

cd "$ui" || exit 0
if pnpm generate:api >/dev/null 2>&1; then
  mkdir -p "$(dirname "$stamp")"
  printf '%s\n' "$current" > "$stamp"
fi
