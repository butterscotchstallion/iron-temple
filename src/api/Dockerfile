# iron-temple API — Go 1.26 (chi + pgx). Build context is src/api/.
#
# The module is vendored (src/api/vendor), so the build runs fully offline —
# CGO off + vendored deps produce a static binary with no libc dependency.
FROM golang:1.26 AS build
# Release version, baked into the binary (main.version) and surfaced by /health.
# Passed by release.yml (--opt build-arg:VERSION=vX.Y.Z); "dev" for local builds.
ARG VERSION=dev
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOFLAGS=-mod=vendor \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/server ./cmd/server

# scratch: nothing to pull, minimal attack surface. The server binary migrates
# the DB on boot (embedded migrations) then serves on :8080. It talks to Postgres
# with sslmode=require (encrypt, no cert verify) so no CA is strictly needed, but
# the public CA bundle is copied in cheaply in case verified TLS is ever wanted.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/server /server
# Non-root, numeric (scratch has no /etc/passwd). Binds :8080 (>1024, allowed).
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/server"]
