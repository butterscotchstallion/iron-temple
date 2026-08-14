<script lang="ts">
  import { DropdownMenu } from "bits-ui";
  import { push } from "svelte-spa-router";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import LogIn from "@lucide/svelte/icons/log-in";
  import LogOut from "@lucide/svelte/icons/log-out";
  import Settings from "@lucide/svelte/icons/settings";
  import Avatar from "./Avatar.svelte";
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
      <span class="max-w-[10rem] truncate font-semibold">
        {auth.me.displayName || auth.me.username}
      </span>
      <ChevronDown class="size-4 opacity-70" aria-hidden="true" />
    </DropdownMenu.Trigger>

    <DropdownMenu.Portal>
      <DropdownMenu.Content
        sideOffset={8}
        align="end"
        class="z-50 min-w-44 rounded-md border border-border/60 bg-card p-1 shadow-lg shadow-black/40 backdrop-blur"
      >
        <DropdownMenu.Item class={itemClass} onSelect={() => go("/profile")}>
          <Settings class="size-4" aria-hidden="true" />
          Configure profile
        </DropdownMenu.Item>
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
