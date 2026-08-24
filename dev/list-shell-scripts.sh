#!/usr/bin/env sh
# Print the shell scripts this repo owns, one path per line, relative to the repo
# root. Used by BOTH the local shellcheck gate (scripts/preflight.sh) and the CI one
# (.gitea/workflows/shellcheck.yml), so the two can never lint different file sets —
# the same one-definition-two-callers arrangement dev/integration-test.sh has.
#
# WHY THIS ISN'T JUST A GLOB
#
# `*.sh` would miss .gitea/scripts/* — four scripts with no extension, and the ones
# whose bugs are most expensive (they decide which CI jobs run at all). So the walk
# takes any tracked file that either carries a shell extension or opens with a shell
# shebang. A new extensionless helper is picked up the day it's committed, without
# anyone remembering to extend a pattern; a glob that silently stops matching is the
# no-op-gate failure this repo keeps designing against.
#
# Scope is `git ls-files`, which gets two exclusions for free: .git/ and everything
# git-ignored (src/ui/node_modules, the generated hey-api client). src/api/vendor is
# excluded explicitly — those are third-party scripts we don't control and can't fix.
set -eu

# Resolve the repo root from this script's own location (it lives in <root>/dev/), so
# the output doesn't depend on the caller's working directory.
script_dir=$(unset CDPATH; cd -- "$(dirname -- "$0")" && pwd)
root=$(unset CDPATH; cd -- "$script_dir/.." && pwd)
cd "$root"

git ls-files -- ':(exclude)src/api/vendor' | while IFS= read -r file; do
  # ls-files lists paths deleted from the working tree but not yet staged.
  [ -f "$file" ] || continue

  case "$file" in
    *.sh | *.bash) printf '%s\n' "$file"; continue ;;
  esac

  # No extension: sniff the shebang. `|| true` because read reports failure on a
  # file whose only line has no trailing newline — it has still filled $line.
  line=''
  IFS= read -r line < "$file" 2>/dev/null || true
  case "$line" in
    # "…sh" at end of the interpreter path (sh, bash, dash, zsh), or followed by a
    # separator so `#!/bin/bash -e` counts. The second pattern is what keeps
    # `#!/opt/shiny/tool` out.
    '#!'*sh | '#!'*sh[!a-zA-Z0-9_]*) printf '%s\n' "$file" ;;
  esac
done
