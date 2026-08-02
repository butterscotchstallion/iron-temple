# Iron Temple — UI

Svelte 5 + Vite + TypeScript + Tailwind v4. Mobile-first (tuned for iPad),
synthwave aesthetic. The typed API client is generated from the backend's
OpenAPI spec with [hey-api](https://heyapi.dev).

## Setup
```sh
npm install
npm run generate:api   # generates src/lib/api/ from ../api/openapi.yaml
npm run dev            # http://localhost:5173 (proxies /api -> localhost:8080)
```

## Scripts
| Script | Purpose |
|---|---|
| `npm run dev` | Vite dev server |
| `npm run build` | Production build |
| `npm run check` | `svelte-check` type checking |
| `npm run generate:api` | Regenerate the hey-api client from the OpenAPI spec |
| `npm run test:unit` | Vitest unit tests |
| `npm run test:e2e` | Playwright end-to-end tests |

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
move quickly — if `npm install` or `generate:api` complain, check the pinned
versions in `package.json` and hey-api's current config format.
