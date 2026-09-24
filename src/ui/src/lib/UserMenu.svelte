<script lang="ts">
  import { DropdownMenu } from "bits-ui";
  import { push } from "svelte-spa-router";
  import BarChart3 from "@lucide/svelte/icons/bar-chart-3";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import LogIn from "@lucide/svelte/icons/log-in";
  import LogOut from "@lucide/svelte/icons/log-out";
  import Medal from "@lucide/svelte/icons/medal";
  import Rss from "@lucide/svelte/icons/rss";
  import Settings from "@lucide/svelte/icons/settings";
  // Fake grass. The one place in the app that gets to be honest about what the
  // generated lifters are, because it is only ever rendered for the owner.
  import Sprout from "@lucide/svelte/icons/sprout";
  import Users from "@lucide/svelte/icons/users";
  // Distinct from Users above, which marks "Manage accounts". Two entries that
  // both concern people need two silhouettes, or the menu reads as one item
  // repeated.
  import UsersRound from "@lucide/svelte/icons/users-round";
  import Avatar from "./Avatar.svelte";
  import LifterName from "./LifterName.svelte";
  import { auth, signOut } from "./auth.svelte";

  // The header's right-hand side: a sign-in link when signed out, and the
  // avatar + name opening a menu when signed in.
  //
  // Built on bits-ui's DropdownMenu rather than a hand-rolled popover so the
  // keyboard and focus behaviour (Escape, arrow keys, focus return, outside
  // click) comes for free and matches the alert-dialog already in use.

  let open = $state(false);

  function go(path: string) {
    open = false;
    push(path);
  }

  async function handleSignOut() {
    open = false;
    await signOut();
  }

  const itemClass =
    "flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-foreground outline-none transition data-highlighted:bg-primary data-highlighted:text-primary-foreground";
</script>

{#if auth.me}
  <DropdownMenu.Root bind:open>
    <DropdownMenu.Trigger
      class="flex items-center gap-2 rounded-full py-1 pl-1 pr-2 text-sm text-ink transition hover:bg-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      aria-label="Account menu"
    >
      <Avatar user={auth.me} size={28} />
      <LifterName lifter={auth.me} class="max-w-[10rem] font-semibold" crownSize="size-3" />
      <ChevronDown class="size-4 opacity-70" aria-hidden="true" />
    </DropdownMenu.Trigger>

    <DropdownMenu.Portal>
      <!-- No backdrop-blur: bg-card is opaque, so there was never any backdrop
           left to see through it. Same dead effect the scrolling surfaces
           carried — see HeaderBar. -->
      <DropdownMenu.Content
        sideOffset={8}
        align="end"
        class="z-50 min-w-44 rounded-md border border-border/60 bg-card p-1 shadow-lg shadow-black/40"
      >
        <DropdownMenu.Item class={itemClass} onSelect={() => go("/racked")}>
          <BarChart3 class="size-4" aria-hidden="true" />
          Racked
        </DropdownMenu.Item>
        <DropdownMenu.Item class={itemClass} onSelect={() => go("/lifters")}>
          <UsersRound class="size-4" aria-hidden="true" />
          Lifters
        </DropdownMenu.Item>
        <!-- Kept even though Home carries a card for it: that card hides itself
             when the feed is empty, so on a quiet install this is the only way
             to reach the page and read that there is nobody else here yet. -->
        <DropdownMenu.Item class={itemClass} onSelect={() => go("/feed")}>
          <Rss class="size-4" aria-hidden="true" />
          Around the gym
        </DropdownMenu.Item>
        <DropdownMenu.Item class={itemClass} onSelect={() => go("/leaderboard")}>
          <Medal class="size-4" aria-hidden="true" />
          Leaderboard
        </DropdownMenu.Item>
        <DropdownMenu.Item class={itemClass} onSelect={() => go("/profile")}>
          <Settings class="size-4" aria-hidden="true" />
          Configure profile
        </DropdownMenu.Item>
        {#if auth.me.isAdmin}
          <!-- Only the account that claimed the install. Hiding it from everyone
               else is tidiness, not security — the route condition turns the
               hash away and the API answers 403 admin_required regardless. -->
          <DropdownMenu.Item class={itemClass} onSelect={() => go("/admin")}>
            <Users class="size-4" aria-hidden="true" />
            Manage accounts
          </DropdownMenu.Item>
          <!-- Below "Manage accounts" because it is the stranger of the two and
               the ordinary errand should come first. Inside the same isAdmin
               block: one guard for both, so a future entry cannot be added
               outside it by accident. -->
          <DropdownMenu.Item class={itemClass} onSelect={() => go("/astroturfing")}>
            <Sprout class="size-4" aria-hidden="true" />
            Astroturfing
          </DropdownMenu.Item>
        {/if}
        <DropdownMenu.Separator class="my-1 h-px bg-border/60" />
        <DropdownMenu.Item class={itemClass} onSelect={handleSignOut}>
          <LogOut class="size-4" aria-hidden="true" />
          Sign out
        </DropdownMenu.Item>
      </DropdownMenu.Content>
    </DropdownMenu.Portal>
  </DropdownMenu.Root>
{:else if auth.loaded}
  <!-- Held back until the first /me settles, or every page load would flash a
       Sign in link at an already-signed-in user. -->
  <button
    type="button"
    class="flex items-center gap-2 rounded-full px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.2em] text-ink transition hover:bg-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
    onclick={() => push("/signin")}
  >
    <LogIn class="size-4" aria-hidden="true" />
    Sign in
  </button>
{/if}
