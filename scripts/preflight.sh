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
#   --repo  hadolint, gitleaks, shellcheck   (hadolint.yml, secret-scan.yml, shellcheck.yml)
#
# CI-only ON PURPOSE — each needs something this box cannot have:
#   go mod tidy   ignores vendor/ and re-resolves through the Go proxy, which this box
#                 has no route to (and it would mutate go.mod/go.sum). Measured: exit 1,
#                 "permission denied" on the shared read-only module cache. CI checks it.
#                 The proxy itself is healthy and monitored — GOPROXY points at the
#                 in-cluster Athens NodePort, and the connection is refused from this
#                 box while gitea's NodePort connects fine. See docs/sandbox-ui-tooling.md.
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

    # -short skips the integration tests (no TEST_DATABASE_URL, no container runtime).
    gate "go test -short" sh -c "cd '$root/src/api' && go test -short $race -count=1 ./..."

    # Integration suite against a real Postgres. `it-testdb` is baked into the iron-temple-cc
    # devcontainer image: it starts a loopback server (initdb'd at image build time), takes a
    # lock, recreates the database, exports TEST_DATABASE_URL, then runs the command. CI does
    # the equivalent with a throwaway pod — the host-executor runner has no container runtime
    # for Testcontainers, so neither side uses it.
    #
    # Both sides run dev/integration-test.sh, so the flags cannot drift; db/parity_test.go
    # then asserts the server itself matches. Until this gate existed, a DB-layer break was
    # first seen in CI. See homelab-gitops docs/sandbox/iron-temple-postgres-plan.md.
    #
    # The two integration packages' unit tests run twice (once above under -short). CI does
    # the same; keeping the flags identical to CI's is worth the seconds.
    if command -v it-testdb >/dev/null 2>&1; then
      gate "go test (integration)" it-testdb run -- sh "$root/dev/integration-test.sh"
    else
      unavailable "go test (integration)" \
        "  it-testdb is not on PATH — it ships in the iron-temple-cc devcontainer image.
  Outside that sandbox, CI runs this gate."
    fi
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
    # lockfile refresh is caught here instead of in CI. Doubles as the guarantee
    # that node_modules exists before the gates that need it. ~1s warm, ~4s cold.
    #
    # --offline is a DELIBERATE DEVIATION: ui.yml runs --prefer-offline, which uses
    # the store first and falls back to the registry. That fallback does not exist
    # here — the configured registry (pnpm config get registry) refuses the
    # connection from the sandbox, so --prefer-offline would reach for it, fail
    # anyway, and report a registry error instead of the real cause. --offline
    # fails fast and says plainly that the package is not in the baked store.
    #
    # The cost: this gate is stricter than CI, the only one that is. If a
    # dependency lands on main and the sandbox image is not rebuilt, the stale
    # baked store fails this gate on EVERY UI commit while CI stays green. The fix
    # is to rebuild the image — not --no-verify, which costs you every other gate.
    gate "pnpm install --frozen-lockfile --offline" \
      sh -c "cd '$root/src/ui' && pnpm install --frozen-lockfile --offline"
    # generate:api first — svelte-check imports the git-ignored hey-api client.
    gate "pnpm generate:api" sh -c "cd '$root/src/ui' && pnpm generate:api"
    gate "pnpm check"        sh -c "cd '$root/src/ui' && pnpm check"
    gate "pnpm test:unit"    sh -c "cd '$root/src/ui' && pnpm test:unit"
  fi
fi

# ============== repo-wide (Dockerfiles, secrets, shell scripts) ==============
if [ "$run_repo" -eq 1 ]; then
  # Same discovery shellcheck.yml uses — dev/list-shell-scripts.sh is shared by both
  # callers, so this gate and CI's can't drift onto different file sets. Default
  # severity (style and up) on both sides; the tree is clean at that level, and the
  # cheapest moment to keep it there is before the commit.
  #
  # The VERSIONS do differ, deliberately: the devcontainer installs Debian's shellcheck
  # (0.9.0) and the runner image bakes the current upstream release, which flags more.
  # So this gate can pass while CI's finds something — the second one-way divergence
  # here after `pnpm --offline` below, and in the same direction (CI stricter). Such a
  # finding is real; fix the script rather than reaching for --no-verify.
  # The discovery script emits paths relative to the repo root (its documented
  # contract, and what keeps CI's log readable). Absolutise them here: preflight can be
  # invoked from any directory, and shellcheck resolves its arguments against the
  # CALLER's cwd, so the bare list would miss every file from anywhere but the root —
  # a false FAIL, not a clean error. hadolint below is CWD-independent for the same
  # reason, via `find "$root"`; gitleaks gets there by cd'ing inside `sh -c`, which is
  # not an option here (the embedded newlines would become command separators).
  shell_scripts=$(sh "$root/dev/list-shell-scripts.sh" | while IFS= read -r f; do
    printf '%s/%s\n' "$root" "$f"
  done)
  if [ -z "$shell_scripts" ]; then
    # NOT a "nothing to check" skip. Unlike Dockerfiles, shell scripts are never absent
    # here — the discovery script is itself one, so it always finds at least itself. An
    # empty list therefore means discovery broke, and skip() would pass even under
    # --strict, leaving the hook green with this gate silently switched off. That is the
    # no-op gate this whole change exists to prevent, so fail in BOTH modes; the
    # workflow's `exit 1` on a zero-length list is the same guard on the CI side.
    printf '\n\033[1m==> shellcheck\033[0m  \033[31m(discovery broken)\033[0m\n'
    printf '  dev/list-shell-scripts.sh returned no files, which should be impossible.\n'
    printf '  Refusing to report a pass over zero files.\n'
    printf 'FAIL\tshellcheck\n' >> "$summary"
    failed=1
  elif command -v shellcheck >/dev/null 2>&1; then
    # Unquoted for the same reason as hadolint below: gate() runs "$@", so IFS splits
    # the newline-separated list into one argument per file.
    # shellcheck disable=SC2086
    gate "shellcheck" shellcheck $shell_scripts
  else
    unavailable "shellcheck" "  shellcheck is not on PATH."
  fi

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
