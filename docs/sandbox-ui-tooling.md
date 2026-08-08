# Making the UI gates runnable in the sandbox

This documents *why* `scripts/preflight.sh` can't run the frontend gates in the
homelab devcontainer sandbox, and the two ways to fix it. The fix lives in the
**homelab image**, not in this repo — so this is a proposal for the operator to
apply there, plus everything they need to do it.

## The problem

The running sandbox has **no network route to any package registry**. Every one
of these refuses the connection almost instantly:

- `registry.npmjs.org` (public npm)
- the in-cluster npm mirror at `http://192.168.3.10:30507/` (what `npm` is
  configured to use via `NPM_CONFIG_REGISTRY`)
- the in-cluster Go proxy at `http://192.168.3.10:30506/`

The backend copes because its module is vendored (`src/api/vendor/`) and the Go
toolchain + `golangci-lint` are baked into the image — nothing is fetched at run
time. The frontend has no equivalent: `src/ui/node_modules` is git-ignored, `pnpm`
isn't installed, and neither `corepack` nor `npm` can download anything. So
`pnpm check` / `pnpm test:unit` simply cannot run in the sandbox today.

Note `corepack` won't work even if the mirror were reachable: it ignores
`NPM_CONFIG_REGISTRY` and hardcodes `registry.npmjs.org`. Point it at a mirror
with `COREPACK_NPM_REGISTRY`, or skip it and install pnpm with `npm i -g` (which
does honour `NPM_CONFIG_REGISTRY`).

## Fix A (preferred): let the sandbox reach the npm mirror

If the air-gap is incidental rather than a hard security requirement, the cleanest
fix is to allow sandbox egress to the in-cluster npm mirror (and, while you're at
it, the Go proxy). Then nothing needs baking — `pnpm install` just works at run
time, exactly like CI, and it never goes stale as dependencies change. This also
un-breaks `go mod tidy` and the separate Python project's `pip`/`uv` installs.

Wherever the sandbox's egress is restricted (NetworkPolicy / firewall in the
homelab repo), add an allow rule to the mirror's Service/NodePort:

- npm mirror — host `192.168.3.10`, port `30507`
- Go proxy — host `192.168.3.10`, port `30506` (bonus: fixes `go mod tidy`)

Then in the image (or a `postCreateCommand`), activate the pinned pnpm once:

```sh
# npm is already pointed at the mirror via NPM_CONFIG_REGISTRY.
npm install -g pnpm@9.15.0    # version pinned in src/ui/package.json "packageManager"
```

After that, `scripts/preflight.sh` runs the UI gates in the sandbox with no
changes to this repo.

## Fix B (air-gapped fallback): bake pnpm + a warm store into the image

If the sandbox must stay offline, pre-populate everything the UI gates need at
**image-build time** (which has network) and reconstruct `node_modules` offline
at run time. pnpm's `fetch` + `install --offline` is built for exactly this.

In the devcontainer image build (homelab repo), after checking out this repo:

```dockerfile
# 1. Install the pinned pnpm (build stage has network).
RUN npm install -g pnpm@9.15.0

# 2. Warm the content-addressable store from THIS repo's lockfile. `pnpm fetch`
#    populates the store without building node_modules, keyed only on the lockfile.
RUN cd src/ui && pnpm fetch

# 3. The store now lives under pnpm's store-dir and is baked into the image layer.
#    Confirm/point at a stable location if needed:  pnpm config set store-dir ...
```

At run time (offline), `scripts/preflight.sh` will find `pnpm` on PATH but no
`node_modules`, and skip. To make it *run*, reconstruct from the baked store
first — either in a `postCreateCommand` or by extending the preflight's UI branch:

```sh
cd src/ui && pnpm install --offline --frozen-lockfile
```

`--offline` forbids any network and installs purely from the baked store, so it
succeeds air-gapped as long as the store covers the lockfile.

### The catch: staleness

The baked store is a snapshot of `src/ui/pnpm-lock.yaml` at image-build time. When
this repo's UI dependencies change, the image's store no longer covers the new
lockfile and `--offline` fails for the missing packages. So Fix B requires the
sandbox image to be **rebuilt whenever the UI lockfile changes** (a Renovate bump,
a new dependency). Fix A has no such coupling — prefer it if the network policy
can accommodate it.

## Scope note

Only `pnpm check` and `pnpm test:unit` need to run in preflight. The Playwright
**e2e** suite (`pnpm test:e2e`) is deliberately out of scope — it needs the
Firefox browser binary and system libs, and it's covered by CI. Neither fix above
needs to install Playwright browsers.
