#!/usr/bin/env sh
# preflight: run the CI gates locally BEFORE pushing, so a PR doesn't fail on
# something you could have caught in seconds. Mirrors .gitea/workflows/{go,ui}.yml.
#
# Tuned for the homelab devcontainer sandbox, which has NO network to any package
# registry (the npm mirror and Go proxy both refuse connections). That splits the
# gates in two:
#
#   • Backend (src/api) — runs FULLY OFFLINE. The module is vendored (src/api/vendor)
#     and the Go toolchain + golangci-lint are baked into the sandbox image, so
#     vet / lint / unit tests need nothing from the network.
#
#   • Frontend (src/ui) — needs `pnpm` + an installed node_modules, neither of which
#     can be fetched in the sandbox. When they're present (e.g. on a networked
#     laptop) the UI gates run; in the sandbox they are SKIPPED with a notice, and
#     CI is the backstop. See docs/sandbox-ui-tooling.md for the image-level fix.
#
# `go mod tidy` is intentionally NOT run here: it ignores vendor/, needs the (dead)
# Go proxy, and would mutate go.mod/go.sum. CI checks it; don't duplicate it offline.
#
# Exit status: non-zero if any gate that actually RAN failed. Skipped gates never
# fail the run.
#
# Usage:
#   scripts/preflight.sh          # everything runnable here
#   scripts/preflight.sh --api    # backend gates only
#   scripts/preflight.sh --ui     # frontend gates only
set -u

# --- resolve repo root (this script lives in <root>/scripts/) -----------------
script_dir=$(unset CDPATH; cd -- "$(dirname -- "$0")" && pwd)
root=$(unset CDPATH; cd -- "$script_dir/.." && pwd)

# --- what to run --------------------------------------------------------------
run_api=1
run_ui=1
case "${1:-}" in
  --api) run_ui=0 ;;
  --ui)  run_api=0 ;;
  "")    ;;
  *) echo "usage: $0 [--api|--ui]" >&2; exit 2 ;;
esac

# --- tiny reporting harness ---------------------------------------------------
# Each gate appends one "STATUS<TAB>label" line to $summary; the final report and
# the exit code are derived from it. STATUS is PASS | FAIL | SKIP.
summary=$(mktemp)
trap 'rm -f "$summary"' EXIT INT TERM
failed=0

# gate <label> <command...> — run a command, stream its output, record the result.
gate() {
  label=$1; shift
  printf '\n\033[1m==> %s\033[0m\n' "$label"
  if "$@"; then
    printf 'PASS\t%s\n' "$label" >> "$summary"
  else
    printf 'FAIL\t%s\n' "$label" >> "$summary"
    failed=1
  fi
}

skip() { printf 'SKIP\t%s\n' "$1" >> "$summary"; printf '\n\033[1m==> %s\033[0m  (skipped)\n%s\n' "$1" "$2"; }

# ============================ backend (src/api) ==============================
if [ "$run_api" -eq 1 ]; then
  if [ -f "$root/src/api/go.mod" ]; then
    # -mod=vendor: use the committed vendor/ tree; never touch the network.
    GOFLAGS=-mod=vendor
    export GOFLAGS

    # -race needs a C compiler; match CI when we can, fall back gracefully.
    race=""
    if command -v gcc >/dev/null 2>&1 || command -v cc >/dev/null 2>&1; then
      race="-race"
    fi

    gate "go vet"        sh -c "cd '$root/src/api' && go vet ./..."
    if command -v golangci-lint >/dev/null 2>&1; then
      gate "golangci-lint" sh -c "cd '$root/src/api' && golangci-lint run"
    else
      skip "golangci-lint" "  golangci-lint not on PATH — CI runs it."
    fi
    # -short skips the Testcontainers integration tests (they need a container runtime).
    gate "go test -short" sh -c "cd '$root/src/api' && go test -short $race -count=1 ./..."
  else
    skip "backend (src/api)" "  no src/api/go.mod — nothing to check."
  fi
fi

# =========================== frontend (src/ui) ===============================
if [ "$run_ui" -eq 1 ]; then
  if [ ! -d "$root/src/ui" ]; then
    skip "frontend (src/ui)" "  no src/ui — nothing to check."
  elif ! command -v pnpm >/dev/null 2>&1; then
    skip "frontend (src/ui)" "  pnpm is not installed (the sandbox can't fetch it).
  Run these on a networked machine, or see docs/sandbox-ui-tooling.md. CI covers them."
  elif [ ! -d "$root/src/ui/node_modules" ]; then
    skip "frontend (src/ui)" "  src/ui/node_modules is missing — run 'pnpm install' first
  (needs network; impossible in the sandbox). CI covers these."
  else
    # generate:api first — svelte-check imports the git-ignored hey-api client.
    gate "pnpm generate:api" sh -c "cd '$root/src/ui' && pnpm generate:api"
    gate "pnpm check"        sh -c "cd '$root/src/ui' && pnpm check"
    gate "pnpm test:unit"    sh -c "cd '$root/src/ui' && pnpm test:unit"
  fi
fi

# ================================ report =====================================
printf '\n\033[1m===== preflight summary =====\033[0m\n'
while IFS='	' read -r status label; do
  case "$status" in
    PASS) printf '  \033[32m✓ PASS\033[0m  %s\n' "$label" ;;
    FAIL) printf '  \033[31m✗ FAIL\033[0m  %s\n' "$label" ;;
    SKIP) printf '  \033[33m- SKIP\033[0m  %s\n' "$label" ;;
  esac
done < "$summary"

if [ "$failed" -ne 0 ]; then
  printf '\n\033[31mpreflight failed — fix the above before pushing.\033[0m\n'
  exit 1
fi
printf '\n\033[32mpreflight passed.\033[0m Skipped gates (if any) are covered by CI.\n'
exit 0
