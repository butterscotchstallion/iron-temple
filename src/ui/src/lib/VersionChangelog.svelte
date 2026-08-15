<script lang="ts">
  import { LinkPreview } from "bits-ui";
  import changelog from "virtual:iron-temple/changelog";

  // The header's version label, and — when we have release notes for it — a panel
  // listing what shipped in it. Before this the version was inert text: you could
  // see which build was deployed but not what was in it.
  //
  // The notes are baked into the bundle at build time by changelogVirtualModule()
  // in vite.config.ts, which bottoms out in scripts/changelog.sh — the same
  // definition that fills the Gitea Release body. Nothing is fetched at runtime.
  //
  // Built on bits-ui's LinkPreview for the same reason UserMenu.svelte is built on
  // DropdownMenu: positioning, Escape, outside-click and focus handling come for
  // free rather than being hand-rolled.

  let {
    version,
    environment,
    // Defaults to the build's own notes. Injectable so tests can drive the panel
    // from fixtures instead of the checkout's commit history.
    notes = changelog,
  }: {
    version: string;
    environment: string;
    notes?: { version: string; entries: string[] };
  } = $props();

  let open = $state(false);

  // The hint below is referenced rather than nested in the button, so the
  // trigger's text content stays exactly the version string that's on screen.
  const hintId = $props.id();

  const label = $derived(
    version ? `iron-temple ${version}${environment ? `-${environment}` : ""}` : "",
  );

  // No label means /health hasn't answered — there is nothing to hover. No entries
  // means this build has none to show (a dev checkout, or a release of nothing but
  // chores). Either way, fall back to the plain text the header had before.
  const interactive = $derived(label !== "" && notes.entries.length > 0);

  const labelClass = "truncate text-[0.65rem] uppercase tracking-[0.25em]";
</script>

{#if interactive}
  <LinkPreview.Root bind:open openDelay={150} closeDelay={200}>
    <LinkPreview.Trigger>
      {#snippet child({ props })}
        <!-- Rendered as a button, not LinkPreview's default anchor: this navigates
             nowhere. The click handler is what makes the panel reachable on a
             touch screen — LinkPreview's own pointer handlers return early on
             `isTouch(e)`, and its focus handler requires focus-visible, so hover
             and keyboard work out of the box but a tap would do nothing. -->
        <button
          {...props}
          type="button"
          data-testid="version"
          aria-describedby={hintId}
          class="{labelClass} cursor-pointer rounded-sm text-ink/50 decoration-dotted underline-offset-4 transition hover:text-ink/80 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary data-[state=open]:text-ink/80"
          onclick={() => (open = !open)}
        >
          {label}
        </button>
      {/snippet}
    </LinkPreview.Trigger>

    <!-- A description, not part of the button's name: the version is what's on
         screen and what the e2e suite asserts, so it stays the whole of the
         trigger's text. sr-only is position:absolute, so it costs no layout. -->
    <span id={hintId} class="sr-only">Show the release notes for this version</span>

    <LinkPreview.Portal>
      <LinkPreview.Content
        side="bottom"
        align="start"
        sideOffset={8}
        collisionPadding={16}
        data-testid="changelog-panel"
        class="z-50 max-h-[60vh] w-[min(28rem,calc(100vw-2rem))] overflow-y-auto rounded-md border border-border/60 bg-card p-4 shadow-lg shadow-black/40 backdrop-blur"
      >
        <h2 class="text-[0.65rem] font-semibold uppercase tracking-[0.25em] text-primary">
          What's new in {notes.version}
        </h2>
        <ul class="mt-3 space-y-2 text-sm text-ink/80">
          {#each notes.entries as entry}
            <li class="flex gap-2">
              <span class="select-none text-primary/70" aria-hidden="true">&rsaquo;</span>
              <span>{entry}</span>
            </li>
          {/each}
        </ul>
      </LinkPreview.Content>
    </LinkPreview.Portal>
  </LinkPreview.Root>
{:else}
  <span class="{labelClass} text-ink/50" data-testid="version">{label}</span>
{/if}
