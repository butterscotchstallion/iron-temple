#!/usr/bin/env sh
# preflight: run the CI gates locally BEFORE pushing, so a PR doesn't fail on
# something you could have caught in seconds. This is the single definition of
# "the gates" — dev/precommit.sh (the lefthook hook) calls it too, so the hook and
# a manual run can't drift apart.
#
# WHAT IT MIRRORS, AND WHAT IT DELIBERATELY DOESN'T
#
# Aligned with CI (.gitea/workflows/):
#   --api   go vet, golangci-lint, gosec, go test -short -race      (go.yml)
#   --ui    frozen-lockfile install, generate:api, check, test:unit (ui.yml)
#   --repo  hadolint, gitleaks                          (hadolint.yml, secret-scan.yml)
#
# CI-only ON PURPOSE — each needs something this box cannot have:
#   go mod tidy   ignores vendor/ and re-resolves through the Go proxy, which is dead
#                 here (and it would mutate go.mod/go.sum). Measured: exit 1,
#                 "permission denied" on the shared read-only module cache. CI checks it.
#   govulncheck   fetches the vulnerability DB from vuln.go.dev on every run. Measured:
#                 exit 1, i/o timeout. Network-dependent by nature, so it can't be a
#                 local gate on an air-gapped box — same bucket as trivy.
#   trivy         same reason: needs a live vulnerability DB.
#   deploy manifests  kustomize + kubeconform aren't in the image.
#   UI e2e        Playwright builds and serves the app; too slow for a commit hook.
#                 NOTE this also means `pnpm build` is never exercised locally — in
#                 ui.yml the production build is covered only by e2e's preview server.
#
# Exit status: non-zero if any gate that actually RAN failed. Skipped gates never fail
# the run — unless --strict, which turns "the tooling for this gate isn't installed"
# from a SKIP into a FAIL. The pre-commit hook passes --strict: a hook that silently
# no-ops when a tool is missing gives the same green light as one that ran clean.
# Interactive use stays lenient, so a box missing one tool can still check the rest.
#
# Usage:
#   scripts/preflight.sh                  # everything runnable here
#   scripts/preflight.sh --api            # backend gates only
#   scripts/preflight.sh --ui             # frontend gates only
#   scripts/preflight.sh --repo           # Dockerfile lint + secret scan only
#   scripts/preflight.sh --api --repo     # selectors combine
#   scripts/preflight.sh --strict         # missing tooling fails instead of skipping
set -u

# --- resolve repo root (this script lives in <root>/scripts/) -----------------
script_dir=$(unset CDPATH; cd -- "$(dirname -- "$0")" && pwd)
root=$(unset CDPATH; cd -- "$script_dir/.." && pwd)

# --- what to run --------------------------------------------------------------
# Selectors are additive; naming none runs all three.
run_api=0
run_ui=0
run_repo=0
strict=0
selected=0
for arg in "$@"; do
  case "$arg" in
    --api)    run_api=1;  selected=1 ;;
    --ui)     run_ui=1;   selected=1 ;;
    --repo)   run_repo=1; selected=1 ;;
    --strict) strict=1 ;;
    *) echo "usage: $0 [--api] [--ui] [--repo] [--strict]" >&2; exit 2 ;;
  esac
done
if [ "$selected" -eq 0 ]; then
  run_api=1; run_ui=1; run_repo=1
fi

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

# unavailable <label> <reason> — a gate can't run because its tooling is absent.
# Lenient mode records a SKIP and leans on CI; --strict records a FAIL, because the
# caller (the commit hook) asked for a real gate rather than a best-effort one.
# Distinct from skip(), which is for "there is genuinely nothing here to check".
unavailable() {
  if [ "$strict" -eq 0 ]; then
    skip "$1" "$2
  Skipped — CI runs this gate."
    return
  fi
  printf '\n\033[1m==> %s\033[0m  \033[31m(unavailable)\033[0m\n%s\n' "$1" "$2"
  printf '  Blocked: this gate is required here and its tooling is missing.\n'
  printf "  Install it, or bypass this once with 'git commit --no-verify'.\n"
  printf 'FAIL\t%s\n' "$1" >> "$summary"
  failed=1
}

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

    gate "go vet" sh -c "cd '$root/src/api' && go vet ./..."

    if command -v golangci-lint >/dev/null 2>&1; then
      gate "golangci-lint" sh -c "cd '$root/src/api' && golangci-lint run"
    else
      unavailable "golangci-lint" "  golangci-lint is not on PATH."
    fi

    # Same flags go.yml uses. This is the gate whose absence let a G115 (integer
    # overflow, HIGH) reach main in PR #70 — keep the flags identical to CI's, or a
    # local pass stops implying a CI pass.
    if command -v gosec >/dev/null 2>&1; then
      gate "gosec" sh -c "cd '$root/src/api' && gosec -severity medium -confidence medium ./..."
    else
      unavailable "gosec" "  gosec is not on PATH (bake it into the devcontainer image)."
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
    unavailable "frontend (src/ui)" "  pnpm is not installed."
  else
    # --frozen-lockfile is the lockfile-freshness gate ui.yml runs: it fails when
    # package.json and pnpm-lock.yaml disagree, so a dependency edit without a
    # lockfile refresh is caught here instead of in CI. --offline forbids network,
    # so it rehydrates from the image's baked pnpm store or fails loudly — it can
    # never quietly reach for the unreachable registry. Doubles as the guarantee
    # that node_modules exists before the gates that need it. ~1s warm, ~4s cold.
    gate "pnpm install --frozen-lockfile" \
      sh -c "cd '$root/src/ui' && pnpm install --frozen-lockfile --offline"
    # generate:api first — svelte-check imports the git-ignored hey-api client.
    gate "pnpm generate:api" sh -c "cd '$root/src/ui' && pnpm generate:api"
    gate "pnpm check"        sh -c "cd '$root/src/ui' && pnpm check"
    gate "pnpm test:unit"    sh -c "cd '$root/src/ui' && pnpm test:unit"
  fi
fi

# ===================== repo-wide (Dockerfiles, secrets) ======================
if [ "$run_repo" -eq 1 ]; then
  # Same discovery hadolint.yml uses: our Dockerfiles only, never vendored ones.
  # node_modules is excluded on top of CI's list because it exists only locally —
  # CI lints a fresh checkout that has none, so skipping it MATCHES CI rather than
  # diverging from it.
  dockerfiles=$(find "$root" -name Dockerfile \
    -not -path "$root/.git/*" \
    -not -path "$root/src/api/vendor/*" \
    -not -path "*/node_modules/*" | sort)
  if [ -z "$dockerfiles" ]; then
    skip "hadolint" "  no Dockerfiles — nothing to check."
  elif command -v hadolint >/dev/null 2>&1; then
    # Unquoted on purpose: gate() runs "$@", so IFS splits the newline-separated
    # list into one argument per file — a single hadolint invocation, as CI does.
    # (Do NOT route this through `sh -c`: the embedded newlines would be parsed as
    # command separators, and every file after the first runs as its own command.)
    # shellcheck disable=SC2086
    gate "hadolint" hadolint $dockerfiles
  else
    unavailable "hadolint" "  hadolint is not on PATH."
  fi

  # Same invocation secret-scan.yml runs. A secret caught here never enters git
  # history at all; one caught in CI is already pushed and needs a history rewrite.
  if command -v gitleaks >/dev/null 2>&1; then
    gate "gitleaks" \
      sh -c "cd '$root' && gitleaks detect --source . --redact --no-git --config .gitleaks.toml"
  else
    unavailable "gitleaks" "  gitleaks is not on PATH (bake it into the devcontainer image)."
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
