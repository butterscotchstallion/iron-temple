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
RUN pnpm generate:api   # openapi-ts → src/lib/api (generated; gitignored)
RUN pnpm build          # vite build → /app/ui/dist

# Serve the static build with a non-root nginx (uid 101, listens on :8080).
# The UI calls the API at the RELATIVE path /api/v1, so /api/* never reaches this
# container — Traefik path-routes it to the API service.
FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY src/ui/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/ui/dist /usr/share/nginx/html
EXPOSE 8080
