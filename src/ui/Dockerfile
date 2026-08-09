# iron-temple UI — Svelte + Vite SPA. Build context is src/ui/.
#
# NOTE: unlike the API (vendored, offline), this stage needs npm-registry access
# at build time (buildkitd egress, or point pnpm at the in-cluster package-proxy).
FROM node:22-alpine AS build
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@9.15.0 --activate
# Install against the committed lockfile first for better layer caching.
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm build   # vite build → /app/dist

# Serve the static build with a non-root nginx (uid 101, listens on :8080).
# The UI calls the API at the RELATIVE path /api/v1, so /api/* never reaches this
# container — Traefik path-routes it to the API service. This only serves static
# assets, with a SPA fallback for client-side routing.
FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
