#!/usr/bin/env sh
# Refresh the git-ignored hey-api client whenever the working tree moves.
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
# Two deliberate choices:
#
#   • Unconditional, rather than regenerating only when openapi.yaml changed.
#     Generation is ~250ms against a local file. A conditional would need to be
#     right about ORIG_HEAD for three different hooks, and a path predicate that
#     misses a case silently reintroduces exactly the staleness this prevents.
#
#   • Non-fatal. A post-hook that can block a checkout is worse than a stale
#     client, and it is not the gate — dev/precommit.sh and CI are. This is a
#     convenience that keeps the editor honest.
set -eu

root=$(git rev-parse --show-toplevel)

# Fresh clone: no node_modules, so openapi-ts isn't installed yet. Bail quietly —
# `pnpm install` followed by any generate-chained script covers this case.
[ -d "$root/src/ui/node_modules" ] || exit 0

cd "$root/src/ui" || exit 0
pnpm generate:api >/dev/null 2>&1 || true
