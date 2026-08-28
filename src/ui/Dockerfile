# iron-temple UI — Svelte + Vite SPA.
#
# BUILD CONTEXT IS THE REPO ROOT (not src/ui), because the typed API client is
# generated from the backend spec: openapi-ts.config.ts reads ../api/openapi.yaml
# and writes src/lib/api (which is gitignored, so it must be generated here). We
# lay the spec and the UI out as sibling api/ and ui/ dirs to match that path.
#
# NOTE: needs npm-registry access at build time (buildkitd egress or the in-cluster
# package-proxy). The API image builds offline; this one does not.
FROM node:22-alpine AS build
RUN corepack enable && corepack prepare pnpm@9.15.0 --activate
WORKDIR /app
# Spec first, as ../api/openapi.yaml relative to the ui workdir below.
COPY src/api/openapi.yaml ./api/openapi.yaml
WORKDIR /app/ui
# Install against the committed lockfile first for better layer caching.
COPY src/ui/package.json src/ui/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY src/ui/ ./
# Generate the typed client (openapi-ts → src/lib/api, gitignored) then build.
# One RUN: keeps hadolint happy (DL3059) and drops a layer.
RUN pnpm generate:api && pnpm build

# Serve the static build with a non-root nginx (uid 101, listens on :8080).
# The UI calls the API at the RELATIVE path /api/v1, so /api/* never reaches this
# container — Traefik path-routes it to the API service.
FROM nginxinc/nginx-unprivileged:1.31-alpine@sha256:901e944d1f4fc2bd077e8f5568b98c1f6f8cdacf6b97a87747c43134a339b9a7
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
# hadolint ignore=DL3017
RUN apk --no-cache upgrade
USER 101
COPY src/ui/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/ui/dist /usr/share/nginx/html
EXPOSE 8080
