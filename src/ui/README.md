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
| `pnpm test:unit` | Vitest unit + component tests (jsdom) |
| `pnpm test:coverage` | Vitest with a V8 coverage report (text + `coverage/` html/lcov) |
| `pnpm test:e2e` | Playwright end-to-end tests |

## Layout
- `src/App.svelte` — app shell: loads programs from the API (hey-api client), plus the rest timer.
- `src/lib/programs.ts` — pure view helpers for program data (unit-tested).
- `src/lib/RestTimer.svelte` — 3-minute rest countdown (Svelte 5 runes).
- `src/lib/time.ts` — pure helpers (unit-tested in `time.test.ts`).
- `src/lib/api/` — **generated** hey-api client (git-ignored; run `generate:api`).
- `src/lib/VersionChangelog.svelte` — the header's version label, and the panel of
  this release's notes that opens off it on hover/tap/focus. The notes are inlined
  at build time from `virtual:iron-temple/changelog` (see `changelogVirtualModule()`
  in `vite.config.ts`), which reads CI's `changelog.generated.json` when present and
  otherwise derives them from git via `scripts/changelog.sh` — the same definition
  that fills the Gitea Release body. No notes means no panel, just the plain label.

  In dev the notes are whatever is releasable since the last stable tag, labelled
  `unreleased`. When that range is empty — the usual state right after a release,
  since the tag has moved past those commits — it falls back to
  `changelog.sh --release <last tag>`, so you see what a production build of that
  commit shows instead of an empty panel. To check what the dev server is actually
  serving: `curl -s 'http://localhost:5173/@id/__x00__virtual:iron-temple/changelog'`.
- `src/app.css` — Tailwind import + synthwave `@theme` palette.

## Testing
- **Unit + component** (`pnpm test:unit`): Vitest in a jsdom environment.
  `*.test.ts` files live next to their subject. Pure helpers are tested directly;
  components render via [`@testing-library/svelte`](https://testing-library.com/docs/svelte-testing-library/intro).
  `vitest-setup.ts` wires up jest-dom matchers.
- **Coverage** (`pnpm test:coverage`): V8 provider. The denominator is first-party
  code only — the generated API client (`src/lib/api/`) and the vendored
  shadcn-svelte primitives (`src/lib/components/ui/`) are excluded (see the
  `coverage` block in `vite.config.ts`).
- **e2e** (`pnpm test:e2e`): Playwright on Firefox against the production build.
  The API is mocked with `page.route`, so no backend or DB is needed. The one
  exception is the header's changelog, which is compiled into the bundle rather
  than fetched — `e2e/build-with-changelog.sh` plants a fixture for the build and
  removes it afterwards.

## Notes
- The generated client is git-ignored and produced from `../api/openapi.yaml`;
  regenerate whenever the spec changes (wire this into pre-commit per the design).
  Run `pnpm generate:api` before `pnpm check`/`dev` on a fresh checkout.
- `App.svelte` loads programs via `listPrograms()`; the client's base URL is set
  to `/api/v1` in `main.ts`, which Vite proxies to the Go API in dev.

## Version sensitivity
This scaffold was authored without a reachable npm registry, so nothing here has
been installed or run. Tailwind v4, Svelte 5 (`mount` API, runes), and hey-api
move quickly — if `pnpm install` or `pnpm generate:api` complain, check the pinned
versions in `package.json` and hey-api's current config format. hey-api can be
pinned exactly with `pnpm add -D -E @hey-api/openapi-ts@latest`.
