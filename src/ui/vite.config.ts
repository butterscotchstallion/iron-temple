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
const GENERATED_CLIENT = fileURLToPath(new URL("./src/lib/api", import.meta.url));

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
 * Regenerate the hey-api client when the OpenAPI contract changes, mid-session.
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
 * Shells out to `pnpm generate:api` rather than calling openapi-ts directly so
 * there is one definition of how the client is generated (openapi-ts.config.ts),
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
 * Serve the release notes to the app as `virtual:iron-temple/changelog`.
 *
 * A virtual module rather than a generated file on disk: nothing needs to be
 * committed, gitignored, or regenerated before `svelte-check` and vitest can
 * resolve the import, and the data is inlined into the bundle at build time so
 * the panel costs no request at runtime.
 */
function changelogVirtualModule(): Plugin {
  const resolvedId = `\0${CHANGELOG_MODULE}`;

  return {
    name: "iron-temple:changelog",
    resolveId(id) {
      return id === CHANGELOG_MODULE ? resolvedId : null;
    },
    load(id) {
      if (id !== resolvedId) return null;
      return `export default ${JSON.stringify(readChangelog())};`;
    },
    // Recompute when the checkout moves. load() runs once and Vite caches the
    // result for the life of the dev server, so without this the panel is
    // frozen at whatever git said when `pnpm dev` started: pull a release and
    // the header still shows the previous one's notes — or, if the release tag
    // hasn't been fetched yet, labels the new commits "unreleased" — until the
    // server is restarted. Nobody restarts a dev server to check a changelog,
    // so it just reads as broken.
    configureServer(server) {
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
        changeOrigin: false,
      },
    },
  },
  test: {
    // jsdom so we can render components; globals for testing-library auto-cleanup.
    environment: "jsdom",
    globals: true,
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
