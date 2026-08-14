#!/usr/bin/env sh
# The API integration suite — defined ONCE, run by both CI and the local gate.
#
# Callers:
#   .gitea/workflows/go.yml   (integration job)  — TEST_DATABASE_URL → a throwaway pod
#   scripts/preflight.sh      (--api gate)       — TEST_DATABASE_URL → sandbox loopback PG
#
# The point is that neither caller owns the flags. Before this existed, the workflow had
# the invocation inline and the local gate had nothing; the moment both sides run the suite,
# a flag that drifts between them turns a green local run into a red PR — the exact failure
# the local gate is supposed to remove. Same "one declaration, two consumers" shape as
# homelab-gitops' scripts/checks.py.
#
# How the database gets there is deliberately NOT this script's business: CI provisions a
# pod (the host-executor runner has no container runtime for Testcontainers), the sandbox
# starts a loopback server via `it-testdb run`. Both just set TEST_DATABASE_URL, which
# db/migrate_test.go and internal/api/integration_test.go already honour.
#
# Server-side parity (version / encoding / collation) is asserted by db/parity_test.go
# rather than here: it runs inside the suite, in the same connection, so it needs no psql
# on either side — the CI runner image has no postgresql-client — and it cannot be
# bypassed by a caller that invokes `go test` directly.
#
# Background: homelab-gitops docs/sandbox/iron-temple-postgres-plan.md
set -eu

: "${TEST_DATABASE_URL:?TEST_DATABASE_URL must be set (CI sets a pod IP; locally use \`it-testdb run\`)}"

# Pinned on BOTH sides rather than inherited from whatever the runner/container defaults to.
# Every timestamp column is TIMESTAMPTZ so comparisons are already absolute — this removes
# rendered-offset differences as a class instead of relying on two images agreeing.
TZ=UTC
export TZ

cd "$(dirname "$0")/../src/api"

# -p 1        the db and api packages share ONE database — parallel package binaries would
#             run concurrent migrations against it.
# -count=1    no cached results; a "pass" must mean it actually ran.
# no -race    CI has never used it here; adding it locally only would break flag parity.
# not -short  so the tests use TEST_DATABASE_URL instead of self-skipping.
exec go test -p 1 -count=1 ./db/ ./internal/api/
