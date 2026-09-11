# Local-dev PostgreSQL for iron-temple, provisioned like the shared cluster
# instance. postgres:17-alpine matches the Testcontainers image used by tests.
#
# Build context is this dev/ directory:  docker build -t iron-temple-postgres:dev dev/
FROM postgres:18-alpine@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2

# Provision the iron_temple tenant on first container init. The password is
# supplied at runtime via APP_DB_PASSWORD (see docker-compose.yml), not baked in.
COPY initdb/ /docker-entrypoint-initdb.d/
