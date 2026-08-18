<script lang="ts">
  import { link } from "svelte-spa-router";
  import { onMount } from "svelte";
  import ClipboardList from "@lucide/svelte/icons/clipboard-list";
  import Dumbbell from "@lucide/svelte/icons/dumbbell";
  import HistoryIcon from "@lucide/svelte/icons/history";
  import LibraryIcon from "@lucide/svelte/icons/library";
  import TrendingUp from "@lucide/svelte/icons/trending-up";

  // The icon is decoration, not content: every tab keeps its text label, and the
  // icons are aria-hidden so the accessible name stays the label alone.
  const items = [
    { href: "/", label: "Workout", icon: Dumbbell },
    { href: "/programs", label: "Programs", icon: ClipboardList },
    { href: "/library", label: "Library", icon: LibraryIcon },
    { href: "/history", label: "History", icon: HistoryIcon },
    { href: "/progress", label: "Progress", icon: TrendingUp },
  ];

  // svelte-spa-router@5.1.1 does not export a `location` store (importing it fails the
  // build). The router is hash-based, so read the current route from the URL hash
  // directly — routes look like "#/history", and `hashchange` fires on every
  // navigation, including use:link clicks.
  function currentPath(): string {
    return window.location.hash.replace(/^#/, "") || "/";
  }

  let path = $state(currentPath());

  onMount(() => {
    const update = () => (path = currentPath());
    window.addEventListener("hashchange", update);
    return () => window.removeEventListener("hashchange", update);
  });

  // A tab owns its section, including nested detail routes. "Workout" covers
  // the home tab and the active-session flow; the "Programs" tab owns the
  // picker and program detail (/programs, /programs/:id).
  function isActive(href: string, p: string): boolean {
    if (href === "/") {
      return p === "/" || p.startsWith("/sessions");
    }
    return p === href || p.startsWith(href + "/");
  }
</script>

<nav class="mt-5 flex justify-center">
  <!-- flex-wrap, and justify-center on the pills themselves: five tabs no longer
       fit one row on a narrow phone, and a second centred row reads better than
       a bar that overflows its own border. -->
  <div
    class="inline-flex flex-wrap items-center justify-center gap-1 rounded-full border border-border/60 bg-card/40 p-1 backdrop-blur"
  >
    {#each items as item (item.href)}
      {@const Icon = item.icon}
      <a
        href={item.href}
        use:link
        aria-current={isActive(item.href, path) ? "page" : undefined}
        class="inline-flex items-center gap-1.5 rounded-full px-4 py-1.5 text-xs font-semibold uppercase tracking-[0.2em] transition {isActive(
          item.href,
          path,
        )
          ? 'bg-primary text-primary-foreground shadow-[0_0_14px_rgba(176,38,255,0.6)]'
          : 'text-muted-foreground hover:bg-foreground/5 hover:text-foreground'}"
      >
        <Icon class="size-3.5" aria-hidden="true" />
        {item.label}
      </a>
    {/each}
  </div>
</nav>
