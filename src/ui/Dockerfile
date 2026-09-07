# iron-temple UI — Svelte + Vite SPA.
#
# BUILD CONTEXT IS THE REPO ROOT (not src/ui), because the typed API client is
# generated from the backend spec: orval.config.ts reads ../api/openapi.yaml
# and writes src/lib/api/generated (which is gitignored, so it must be generated
# here — the barrel at src/lib/api/index.ts is tracked and arrives with the copy). We
# lay the spec and the UI out as sibling api/ and ui/ dirs to match that path.
#
# NOTE: needs npm-registry access at build time (buildkitd egress or the in-cluster
# package-proxy). The API image builds offline; this one does not.
FROM node:24-alpine@sha256:e67514e5d0f6c46656005e1b693b2ec9d52e80b641307de684d4a015ba7a4eaf AS build
RUN corepack enable && corepack prepare pnpm@9.15.0 --activate
WORKDIR /app
# Spec first, as ../api/openapi.yaml relative to the ui workdir below.
COPY src/api/openapi.yaml ./api/openapi.yaml
WORKDIR /app/ui
# Install against the committed lockfile first for better layer caching.
COPY src/ui/package.json src/ui/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY src/ui/ ./
# Generate the typed client (orval → src/lib/api/generated, gitignored) then build.
# One RUN: keeps hadolint happy (DL3059) and drops a layer.
RUN pnpm generate:api && pnpm build

# Serve the static build with a non-root nginx (uid 101, listens on :8080).
# The UI calls the API at the RELATIVE path /api/v1, so /api/* never reaches this
# container — Traefik path-routes it to the API service.
FROM nginxinc/nginx-unprivileged:1.31-alpine@sha256:aa8c9087d36d93e9d650c5365f883b421e8214aedbad24ade52b844c583358f1
# Patch Alpine base packages (openssl/libcrypto3, c-ares, libexpat, libpng, …) so
# the Trivy CRITICAL/HIGH gate passes — the pinned nginx base tag lags Alpine's
# security releases. Runs as root at build time, then drops back to the non-root
# nginx user. DL3017 (no apk upgrade) is intentionally overridden: freshening the
# base for security is exactly the goal of a Trivy-gated image.
#
# Numeric UIDs throughout (0, not "root"), matching USER 101 below and
# src/api/Dockerfile's 65532: a name has to be resolved against the image's
# /etc/passwd, which a runtime enforcing runAsNonRoot or a rootless engine's
# user namespace may not read the way the image expects. 0 is unambiguous.
USER 0
# CACHEBUST exists because the upgrade below is worthless without it. Its layer's
# cache key is (base digest + the command string) and NEITHER changes between
# releases — so buildkit reuses the layer from the registry cache (release.yml
# --import-cache) and `apk upgrade` never re-runs. The step then reports success
# while shipping whatever Alpine packages were current the day the layer was first
# built. That is how v0.31.1 failed its own Trivy gate on seven HIGH util-linux
# CVEs (libuuid 2.42.1-r0, fixed in 2.42.3-r0) with this upgrade line already
# present: `#18 [stage-1 2/4] RUN apk --no-cache upgrade` → CACHED.
#
# release.yml passes a fresh value per run, so the upgrade always re-runs. It busts
# EVERY build rather than per-day because the Trivy scan here is a release-blocking
# gate: a retry after Alpine publishes a fix has to be able to pick that fix up,
# rather than wedging the release until the date rolls over. It is cheap — only the
# three trailing stage-1 layers rebuild; the expensive `build` stage above is a
# separate stage that never sees this ARG.
#
# DO NOT "tidy away" the ${CACHEBUST} reference in the RUN below. Declaring the ARG
# is not enough: a build arg invalidates cache at its first USE, not its definition
# ("a cache miss occurs upon its first usage, not its definition" — Dockerfile
# reference, Impact on build caching). An ARG the RUN never mentions leaves the
# command string identical, so the layer restores from cache and the upgrade goes
# back to silently not running — the exact bug this whole block exists to fix, but
# now wearing a cache-bust that looks like it works. The echo is the fix.
ARG CACHEBUST=0
# hadolint ignore=DL3017
RUN echo "cachebust: ${CACHEBUST}" && apk --no-cache upgrade
USER 101
COPY src/ui/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/ui/dist /usr/share/nginx/html
EXPOSE 8080
