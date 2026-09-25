import { execFile, execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  statSync,
  watch,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";
import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";

const UI_ROOT = fileURLToPath(new URL(".", import.meta.url));
const OPENAPI_SPEC = fileURLToPath(new URL("../api/openapi.yaml", import.meta.url));
// The generated tree specifically, not src/lib/api — that directory also holds
// the tracked barrel (index.ts) and so exists even when nothing has been
// generated, which would make the "is the client on disk?" check below always
// say yes and let a missing client through as up to date.
const GENERATED_CLIENT = fileURLToPath(new URL("./src/lib/api/generated", import.meta.url));

// Shared with dev/regen-api.sh — same path, same digest, so the hook and the dev
// server never disagree about whether the client is current. The shell side hashes
// via `sha256sum < spec`, which keeps the filename out of the digest and makes the
// two implementations produce identical hex.
const SPEC_STAMP = fileURLToPath(
  new URL("./node_modules/.cache/iron-temple/openapi-spec.sha256", import.meta.url),
);

// Release notes for the running build, baked in at build time. CI writes the JSON
// (see .gitea/workflows/release.yml); locally we derive it from git.
const CHANGELOG_MODULE = "virtual:iron-temple/changelog";
// The same notes again, as a file in the build output. The module above is for
// the build that's running; this one is how a build tells the PREVIOUS one what
// it contains — see loadNotes() in src/lib/version.svelte.ts.
const CHANGELOG_ASSET = "changelog.json";
const CHANGELOG_JSON = fileURLToPath(new URL("./changelog.generated.json", import.meta.url));
const CHANGELOG_SCRIPT = fileURLToPath(new URL("../../scripts/changelog.sh", import.meta.url));
const REPO_ROOT = fileURLToPath(new URL("../../", import.meta.url));

function hashSpec(): string | null {
  try {
    return createHash("sha256").update(readFileSync(OPENAPI_SPEC)).digest("hex");
  } catch {
    return null;
  }
}

function readStamp(): string | null {
  try {
    return readFileSync(SPEC_STAMP, "utf8").trim();
  } catch {
    return null;
  }
}

function writeStamp(hash: string): void {
  try {
    mkdirSync(dirname(SPEC_STAMP), { recursive: true });
    writeFileSync(SPEC_STAMP, `${hash}\n`);
  } catch {
    // A missing stamp only costs a redundant regeneration, so this is not worth
    // interrupting the dev server over.
  }
}

/**
 * Regenerate the orval client when the OpenAPI contract changes, mid-session.
 *
 * `pnpm dev` generates the client once at startup, but the spec lives outside the
 * Vite root and no module imports it, so it is not in the module graph and Vite
 * never watches it. Editing the contract therefore did nothing until you restarted
 * the dev server.
 *
 * Only the regeneration is missing — Vite handles the rest on its own. Writing to
 * src/lib/api/ triggers a reload of the client modules and an HMR update of every
 * route that imports them, because those files ARE in the graph.
 *
 * That reload is also why this compares content hashes rather than regenerating on
 * every save: the generated client is plain .ts with no HMR accept handler, so any
 * rewrite costs a FULL PAGE RELOAD and the in-page state that goes with it. A save
 * that leaves the spec byte-identical — the reflexive ctrl-s, a formatter that
 * changed nothing — must not cost you that.
 *
 * Shells out to `pnpm generate:api` rather than calling orval directly so
 * there is one definition of how the client is generated (orval.config.ts),
 * shared with CI, the pre-commit hook and dev/regen-api.sh.
 */
function regenerateApiOnSpecChange(): Plugin {
  let running = false;
  let queued = false;

  return {
    name: "iron-temple:regenerate-api",
    // Dev server only: `vite build` gets its client from the build script's chain.
    apply: "serve",
    configureServer(server) {
      const { logger } = server.config;

      const regenerate = () => {
        // An editor can emit several change events for one save, and generation is
        // not instant. Collapse concurrent requests into one trailing re-run so the
        // final state always reflects the last write.
        if (running) {
          queued = true;
          return;
        }

        const hash = hashSpec();
        // Unreadable mid-write: the editor's next event will bring us back.
        if (hash === null) return;
        // Byte-identical to what the client was last generated from, and that client
        // is still on disk. Nothing to do, and no page reload inflicted.
        if (hash === readStamp() && existsSync(GENERATED_CLIENT)) return;

        running = true;
        execFile("pnpm", ["generate:api"], { cwd: UI_ROOT }, (error) => {
          running = false;
          if (error) {
            // Usually a half-saved spec that isn't valid YAML yet. Report it and
            // wait for the next save rather than killing the dev server. The stamp
            // is deliberately left alone, so the next valid save regenerates.
            logger.error(
              `[regenerate-api] openapi.yaml changed but generation failed:\n${error.message}`,
              { timestamp: true },
            );
          } else {
            writeStamp(hash);
            logger.info("[regenerate-api] openapi.yaml changed - client regenerated", {
              timestamp: true,
            });
          }
          if (queued) {
            queued = false;
            regenerate();
          }
        });
      };

      server.watcher.add(OPENAPI_SPEC);
      server.watcher.on("change", (file) => {
        if (file === OPENAPI_SPEC) regenerate();
      });
    },
  };
}

type Changelog = { version: string; entries: string[] };

const EMPTY_CHANGELOG: Changelog = { version: "", entries: [] };

// What scripts/changelog.sh prints when nothing releasable landed in the range.
// It reads as a line item but means "no entries", and the header panel hides
// itself entirely rather than showing it.
const NO_NOTABLE_CHANGES = "(no notable changes)";

/** Run scripts/changelog.sh in one of its modes and return its stdout. */
function runChangelog(...args: string[]): string {
  return execFileSync("bash", [CHANGELOG_SCRIPT, ...args], {
    cwd: REPO_ROOT,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "ignore"],
  });
}

/** Turn changelog.sh's `- subject (abc1234)` lines into bare entries. */
function parseNotes(notes: string): string[] {
  return notes
    .split("\n")
    .map((line) => line.replace(/^\s*-\s*/, "").trim())
    .filter((line) => line !== "" && line !== NO_NOTABLE_CHANGES);
}

/**
 * The release notes for this build, from whichever source is available.
 *
 * Both paths bottom out in scripts/changelog.sh — the same definition that fills
 * the Gitea Release body — so the panel in the header and the release page can't
 * disagree about what shipped.
 *
 * Every failure degrades to no entries rather than throwing. The changelog is
 * decoration: a build must not fail because a git command did, and CI's step that
 * produces the JSON is deliberately `continue-on-error`.
 */
function readChangelog(): Changelog {
  // Component tests render against their own fixtures, so reading the real
  // history here would only make the suite depend on the checkout's commits.
  if (process.env.VITEST) return EMPTY_CHANGELOG;

  // CI's copy wins where it exists: inside the UI image build, .dockerignore
  // excludes .git and scripts/, so the JSON is the only source that survives.
  try {
    const parsed: unknown = JSON.parse(readFileSync(CHANGELOG_JSON, "utf8"));
    if (parsed && typeof parsed === "object") {
      const { version, entries } = parsed as Partial<Changelog>;
      return {
        version: typeof version === "string" ? version : "",
        entries: Array.isArray(entries) ? entries.filter((e) => typeof e === "string") : [],
      };
    }
  } catch {
    // Absent (the normal local case) or malformed — fall through to git.
  }

  // Local `pnpm dev`/`pnpm build`: derive it from the working tree.
  try {
    // Anything releasable since the last stable tag is work this checkout has
    // that the tag doesn't, so label it as unreleased rather than borrowing a
    // version that doesn't contain it.
    const pending = parseNotes(runChangelog());
    if (pending.length > 0) return { version: "unreleased", entries: pending };

    // Nothing yet — the usual state of a freshly tagged main, since the release
    // that consumed those commits moved the tag past them. Fall back to that
    // release's own notes, which is what a production build of this commit
    // shows; otherwise the panel is invisible in dev for most of a release
    // cycle and looks broken rather than empty.
    const tag = runChangelog("--last-tag").trim();
    if (!tag) return EMPTY_CHANGELOG;
    return { version: tag, entries: parseNotes(runChangelog("--release", tag)) };
  } catch {
    return EMPTY_CHANGELOG;
  }
}

/**
 * The git directories holding the refs readChangelog() reads through
 * scripts/changelog.sh: HEAD says which commits this checkout has, and the tags
 * say which of them have been released.
 *
 * Asked of git rather than assumed to be `<root>/.git`, because that is only
 * true of a plain clone. In a worktree `.git` is a file, HEAD lives in the
 * worktree's own directory, and the tags live in the main checkout's — so both
 * paths are needed and only git knows them.
 */
function gitRefDirs(): string[] {
  try {
    const out = execFileSync("git", ["rev-parse", "--git-dir", "--git-common-dir"], {
      cwd: REPO_ROOT,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    });
    const dirs = out
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line !== "")
      // Both are printed relative to REPO_ROOT when they sit inside it.
      .map((dir) => resolve(REPO_ROOT, dir));
    return [...new Set(dirs)];
  } catch {
    // Not a checkout (the UI image build, where .dockerignore drops .git). The
    // JSON is the source there and it cannot change under a running server.
    return [];
  }
}

/**
 * Publish the release notes twice: inlined into the bundle as
 * `virtual:iron-temple/changelog`, and written to the build output as
 * `changelog.json`.
 *
 * The module is what the header panel reads (VersionChangelog.svelte). A virtual
 * module rather than a generated file on disk: nothing needs to be committed,
 * gitignored, or regenerated before `svelte-check` and vitest can resolve the
 * import, and the data is inlined at build time so the panel costs no request at
 * runtime.
 *
 * The file exists because the module can only ever describe the build it was
 * compiled into — which is the build you are ALREADY RUNNING. A tab that has been
 * open since before a release has no way to say what is in the release it is being
 * offered. Once the new pod is serving, its changelog.json is that answer, and the
 * update prompt fetches it (src/lib/version.svelte.ts).
 *
 * Both come from one readChangelog() call per build, because the whole point is
 * that the panel and the prompt cannot disagree about what shipped.
 */
function changelogVirtualModule(): Plugin {
  const resolvedId = `\0${CHANGELOG_MODULE}`;

  // Populated on build only. load() and generateBundle() each want the notes,
  // and readChangelog() shells out to changelog.sh — calling it twice would mean
  // two independent answers that could differ, which is exactly what this plugin
  // exists to prevent. Deliberately NOT cached on serve: the invalidation below
  // works by making load() run again, so a cache there would freeze the panel.
  let built: Changelog | undefined;
  let isBuild = false;
  const changelog = () => {
    if (!isBuild) return readChangelog();
    built ??= readChangelog();
    return built;
  };

  // Where the file lands, honouring `base` so the served path and the path the
  // dev middleware answers cannot drift apart if one is ever configured.
  let assetPath = `/${CHANGELOG_ASSET}`;

  return {
    name: "iron-temple:changelog",
    configResolved(config) {
      isBuild = config.command === "build";
      assetPath = `${config.base}${CHANGELOG_ASSET}`;
    },
    resolveId(id) {
      return id === CHANGELOG_MODULE ? resolvedId : null;
    },
    load(id) {
      if (id !== resolvedId) return null;
      return `export default ${JSON.stringify(changelog())};`;
    },
    // `fileName` rather than `name`: `name` is a source name that goes through
    // assetFileNames and comes back fingerprinted under assets/, which is useless
    // to a bundle that has to guess the URL. fileName is written verbatim, so the
    // path stays the one version.svelte.ts asks for.
    generateBundle() {
      this.emitFile({
        type: "asset",
        fileName: CHANGELOG_ASSET,
        source: JSON.stringify(changelog()),
      });
    },
    // Recompute when the checkout moves. load() runs once and Vite caches the
    // result for the life of the dev server, so without this the panel is
    // frozen at whatever git said when `pnpm dev` started: pull a release and
    // the header still shows the previous one's notes — or, if the release tag
    // hasn't been fetched yet, labels the new commits "unreleased" — until the
    // server is restarted. Nobody restarts a dev server to check a changelog,
    // so it just reads as broken.
    configureServer(server) {
      // Serve the same file the build emits, so dev and production agree about
      // what lives at this path. Nothing in dev actually fetches it — isDevBuild()
      // in version.svelte.ts excludes `dev-<sha>` on both sides of the comparison,
      // so the prompt never opens there — but without this the SPA fallback answers
      // with index.html and a 200, which is a far more confusing thing to find on
      // the end of a curl than the notes themselves.
      //
      // Registered here in the body rather than in a returned post-hook so it runs
      // BEFORE vite's history fallback, and matched by exact path rather than
      // `use(path, ...)` because connect matches prefixes and strips them.
      server.middlewares.use((req, res, next) => {
        if ((req.url ?? "").split("?")[0] !== assetPath) return next();
        res.setHeader("Content-Type", "application/json");
        // Matches the header nginx.conf sets in production.
        res.setHeader("Cache-Control", "no-cache");
        // Computed per request, not at registration: this hook also runs under
        // vitest (which starts vite in middleware mode), where readChangelog()
        // short-circuits to empty rather than shelling out to git.
        res.end(JSON.stringify(changelog()));
      });

      const dirs = gitRefDirs();
      if (dirs.length === 0) return;

      let timer: ReturnType<typeof setTimeout> | undefined;

      const refresh = () => {
        // A fetch or a checkout rewrites a burst of refs. Recompute once when
        // it settles rather than once per file, since each recompute shells out
        // to changelog.sh and ends in a page reload.
        clearTimeout(timer);
        timer = setTimeout(() => {
          const mod = server.moduleGraph.getModuleById(resolvedId);
          if (!mod) return;
          server.moduleGraph.invalidateModule(mod);
          // Plain data with no HMR accept handler, so the panel only picks the
          // new notes up on a reload.
          server.ws.send({ type: "full-reload" });
        }, 200);
      };

      // node's fs.watch rather than server.watcher: Vite ignores **/.git/** by
      // default, and lifting that would put every object git writes into the
      // dev server's watch set to catch the handful of files that matter.
      const watchers = dirs.flatMap((dir) =>
        // HEAD covers commit and branch moves; the other three cover a tag or
        // branch arriving, loose (refs/…) or packed (packed-refs).
        ["HEAD", "packed-refs", "refs/heads", "refs/tags"].flatMap((entry) => {
          try {
            const w = watch(join(dir, entry), { persistent: false }, refresh);
            // packed-refs comes and goes with `git gc`, and a watch whose path
            // is removed reports it here rather than throwing above.
            w.on("error", () => {});
            return [w];
          } catch {
            // Absent — packed-refs in a freshly cloned repo, or refs/tags in one
            // with no tags yet. The remaining watches still see it appear.
            return [];
          }
        }),
      );

      server.httpServer?.on("close", () => {
        clearTimeout(timer);
        for (const w of watchers) w.close();
      });
    },
  };
}

/**
 * Tell the browser about the display font while it is still reading the HTML.
 *
 * Orbitron is `--font-display`, and the only thing it styles is the "Iron
 * Temple" H1 — the largest element on the page, and so almost certainly what
 * decides LCP. Self-hosting it (app.css) removed a third-party round trip, but
 * left the file three hops from the document: the HTML has to arrive, the
 * stylesheet has to be fetched and parsed, and only then does the @font-face
 * rule reveal a .woff2 to go and get.
 *
 * A preload collapses that to two. It has to be injected at build time rather
 * than written into index.html by hand because the filename is content-hashed,
 * so only the bundle knows it.
 *
 * `crossorigin` is not optional even though the font is same-origin: fonts are
 * fetched in CORS mode, and a preload whose mode doesn't match the real request
 * is not reused — the browser downloads the file twice and the preload becomes
 * a pessimisation. Chrome warns about exactly this in the console.
 */
function preloadDisplayFont(): Plugin {
  // transformIndexHtml's `this` is not the Rollup plugin context, so there is no
  // this.warn to reach for. Take Vite's own logger while it is on offer.
  let warn = (message: string) => console.warn(message);

  return {
    name: "iron-temple:preload-display-font",
    apply: "build",
    configResolved(config) {
      warn = (message) => config.logger.warn(`[preload-display-font] ${message}`);
    },
    transformIndexHtml(html, ctx) {
      const font = Object.keys(ctx.bundle ?? {}).find(
        (file) => /orbitron.*\.woff2$/.test(file),
      );
      // No match means the font was renamed or dropped. Log it rather than
      // failing the build — a missing preload is slower, not broken — but do
      // say so, because a silent no-op here looks exactly like a working one.
      if (!font) {
        warn("no Orbitron .woff2 in the bundle; skipping the preload hint");
        return html;
      }
      return {
        html,
        tags: [
          {
            tag: "link",
            attrs: {
              rel: "preload",
              as: "font",
              type: "font/woff2",
              href: `/${font}`,
              crossorigin: "",
            },
            injectTo: "head-prepend",
          },
        ],
      };
    },
  };
}

/** Which built files are worth compressing ahead of time. */
const PRECOMPRESS = /\.(js|css|html|svg|json)$/;

/**
 * Write a .gz beside every compressible asset, for nginx's gzip_static.
 *
 * nginx compresses on the fly at gzip_comp_level 1 by default — the fastest,
 * weakest setting, chosen because it runs per request. These files are
 * immutable and built once, so there is no reason to keep paying that: gzip
 * them at level 9 here and nginx serves the result verbatim, spending nothing
 * per request and sending fewer bytes than it would have compressed itself.
 *
 * Uses node's own zlib rather than a plugin dependency, which for "walk the
 * output directory and gzip some files" is the whole implementation.
 *
 * Compressed files smaller than the original are kept; a .gz that came out
 * bigger is deleted, because gzip_static would otherwise serve the larger one.
 * Anything below nginx's own gzip_min_length is skipped for the same reason it
 * skips them: the framing costs more than the saving.
 */
function precompressAssets(): Plugin {
  return {
    name: "iron-temple:precompress",
    apply: "build",
    // After the bundle is on disk, so it covers assets other plugins emitted
    // too, not only the ones Rollup knows about.
    closeBundle: {
      sequential: true,
      async handler() {
        const outDir = fileURLToPath(new URL("./dist", import.meta.url));
        if (!existsSync(outDir)) return;

        let written = 0;
        let saved = 0;
        for (const file of readdirSync(outDir, { recursive: true, encoding: "utf8" })) {
          if (!PRECOMPRESS.test(file)) continue;
          const path = join(outDir, file);
          if (!statSync(path).isFile()) continue;

          const raw = readFileSync(path);
          if (raw.byteLength < 1024) continue;

          const gz = gzipSync(raw, { level: 9 });
          if (gz.byteLength >= raw.byteLength) continue;

          writeFileSync(`${path}.gz`, gz);
          written += 1;
          saved += raw.byteLength - gz.byteLength;
        }
        this.info(
          `precompressed ${written} files, ${(saved / 1024).toFixed(0)} kB smaller on the wire`,
        );
      },
    },
  };
}

export default defineConfig({
  plugins: [
    svelte(),
    tailwindcss(),
    regenerateApiOnSpecChange(),
    changelogVirtualModule(),
    preloadDisplayFont(),
    precompressAssets(),
  ],
  resolve: {
    // $lib alias for shadcn-svelte's generated components (Vite, not SvelteKit).
    alias: {
      $lib: fileURLToPath(new URL("./src/lib", import.meta.url)),
    },
    // Under Vitest, resolve Svelte's *browser* entry so component rendering
    // (mount/runes) works in jsdom. Guarded so the real dev/build stays untouched.
    ...(process.env.VITEST ? { conditions: ["browser"] } : {}),
  },
  server: {
    port: 5173,
    // Proxy API calls to the Go backend during development.
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        // changeOrigin would rewrite Host to localhost:8080 while the browser
        // still sends Origin: http://localhost:5173, and the API's CSRF check
        // rejects a mutation whose Origin and Host disagree — so every POST in
        // development would 403. Leaving it off makes dev genuinely same-origin
        // from the API's point of view, which is what production is too
        // (Traefik path-routes /api and preserves Host). The target is a plain
        // Go server that ignores Host, so nothing needs the rewrite.
        //
        // NOW LOAD-BEARING FOR A SECOND REASON. /api/v1/live is a WebSocket,
        // and its handshake is checked against the same origin rule — but a
        // handshake is a GET, which the CSRF middleware skips, so the socket
        // enforces it directly instead. With changeOrigin on, Host would become
        // localhost:8080 while the browser still sent Origin: localhost:5173,
        // and every socket in development would be refused rather than only
        // every POST.
        changeOrigin: false,
        // Upgrade requests are proxied too, which is what /api/v1/live needs.
        // Without this the handshake gets an ordinary HTTP response, the client
        // reconnects on a backoff forever, and the app quietly falls back to
        // polling — working, but never live, and with nothing saying why.
        ws: true,
      },
    },
  },
  test: {
    // jsdom so we can render components; globals for testing-library auto-cleanup.
    environment: "jsdom",
    globals: true,
    // Run each test file in its own VM context inside a reused worker, rather
    // than in its own freshly-forked process.
    //
    // Standing up a jsdom is the single most expensive thing this suite does —
    // far more than the assertions. Under the default `forks` pool it happens
    // once per FILE: 88 files, ~143s, 53% of the run. vmThreads builds the
    // environment once per worker and gives each file a fresh VM context on
    // top of it, so per-file isolation is kept and the setup is amortised.
    // Measured on the same machine: 294s -> 131s, all 1385 tests passing.
    //
    // NOT `isolate: false`, which Vitest also suggests in that hint and which
    // is the cheaper-looking option. It shares one module registry and one
    // document across every file in a worker; tried here, it failed 99 tests in
    // src/routes alone on leaked state. The isolation is load-bearing.
    pool: "vmThreads",
    // Keep transformed modules on disk (node_modules/.vite, already ignored)
    // instead of recompiling every .svelte and .ts on each run.
    //
    // Compiling the component graph is ~14% of a cold run and ~1% of a warm
    // one: 108s -> 87s here. That is a local-development win specifically —
    // watch mode and the repeated `pnpm test:unit` of the preflight gate — and
    // it does nothing in CI, which starts from a fresh checkout every time. It
    // costs nothing there either; a cold run with this on is the same length as
    // a run without it, so there is no reason to make it conditional.
    fsModuleCache: true,
    // Vitest's default is 5000, and on this machine that is not a timeout, it is a
    // coin toss.
    //
    // The box has TWO CORES and is shared: several agents run gates on it at once,
    // and load routinely sits at 4-7. The heaviest test in the suite —
    // Racked.test.ts's "renders the headline and every populated section", which
    // renders six charts at once — takes well under 5s on an idle box and was
    // measured at 6.1s, 8.2s, 8.3s and 9.7s under contention. It fails on
    // origin/main just as readily as on a branch, so it is nothing any one change
    // introduced.
    //
    // That made it a gate that refused pushes for reasons unrelated to the diff,
    // twice, on two different branches — a red suite that says nothing about the
    // code is worse than a slow one, because it teaches everybody to re-run rather
    // than to read.
    //
    // Fifteen seconds: 3x the default and comfortably above the worst observed.
    // Raised here rather than on the one `it()` because the cause is the machine
    // and not that assertion — the next heavy render would hit exactly the same
    // wall. It does NOT slow a green run: only a test that actually fails waits
    // out its timeout, and a genuine hang still fails, ten seconds later than it
    // used to.
    testTimeout: 15_000,
    setupFiles: ["./vitest-setup.ts"],
    include: ["src/**/*.{test,spec}.ts"],
    coverage: {
      provider: "v8",
      reporter: ["text", "html", "lcov"],
      // First-party code only: the generated API client and the shadcn-svelte
      // UI primitives are vendored/generated, so they'd only dilute the signal.
      include: ["src/**/*.{ts,svelte}"],
      exclude: [
        "src/**/*.{test,spec}.ts",
        // Throwaway components that exist only to drive a test.
        "src/**/*Fixture.svelte",
        "src/lib/api/**",
        "src/lib/components/ui/**",
        "src/main.ts",
        "src/vite-env.d.ts",
        "**/*.d.ts",
      ],
    },
  },
});
