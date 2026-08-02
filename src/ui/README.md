# Iron Temple — UI

Svelte 5 + Vite + TypeScript + Tailwind v4. Mobile-first (tuned for iPad),
synthwave aesthetic. The typed API client is generated from the backend's
OpenAPI spec with [hey-api](https://heyapi.dev).

Package manager: **pnpm** (pinned via `packageManager` in `package.json`; use
`corepack enable` to get the matching version automatically).

## Setup
```sh
pnpm install
pnpm generate:api   # generates src/lib/api/ from ../api/openapi.yaml
pnpm dev            # http://localhost:5173 (proxies /api -> localhost:8080)
```

## Scripts
| Script | Purpose |
|---|---|
| `pnpm dev` | Vite dev server |
| `pnpm build` | Production build |
| `pnpm check` | `svelte-check` type checking |
| `pnpm generate:api` | Regenerate the hey-api client from the OpenAPI spec |
| `pnpm test:unit` | Vitest unit tests |
| `pnpm test:e2e` | Playwright end-to-end tests |

## Layout
- `src/App.svelte` — app shell (currently static program cards + rest timer).
- `src/lib/RestTimer.svelte` — 3-minute rest countdown (Svelte 5 runes).
- `src/lib/time.ts` — pure helpers (unit-tested in `time.test.ts`).
- `src/lib/api/` — **generated** hey-api client (git-ignored; run `generate:api`).
- `src/app.css` — Tailwind import + synthwave `@theme` palette.

## Notes
- The generated client is git-ignored and produced from `../api/openapi.yaml`;
  regenerate whenever the spec changes (wire this into pre-commit per the design).
- `App.svelte` uses placeholder data; swapping in `listPrograms()` from the
  generated client is the next step once the API is running.

## Version sensitivity
This scaffold was authored without a reachable npm registry, so nothing here has
been installed or run. Tailwind v4, Svelte 5 (`mount` API, runes), and hey-api
move quickly — if `pnpm install` or `pnpm generate:api` complain, check the pinned
versions in `package.json` and hey-api's current config format. hey-api can be
pinned exactly with `pnpm add -D -E @hey-api/openapi-ts@latest`.
