<script lang="ts">
  import { DropdownMenu } from "bits-ui";
  import { push } from "svelte-spa-router";
  import Bell from "@lucide/svelte/icons/bell";
  import Avatar from "./Avatar.svelte";
  import { auth } from "./auth.svelte";
  import { relativeTime } from "./date";
  import {
    clearAll,
    markAllRead,
    notifications,
    poll,
  } from "./notifications.svelte";
  import type { Notification } from "./api";

  // The header's notification bell, and the panel behind it.
  //
  // Built on bits-ui's DropdownMenu for the reason UserMenu.svelte is: Escape,
  // outside click, focus return and arrow-key movement come for free and match
  // the menu sitting next to it. There is no hand-rolled popover anywhere in
  // this app and this is not the place to start one.
  //
  // The state is NOT owned here. It lives in notifications.svelte.ts and is
  // polled by App.svelte, because this component is inside the lazy-loaded
  // HeaderBar and must not be what decides whether anybody is counting. Opening
  // the panel therefore draws what the last poll already has, with no spinner
  // for a request that is usually a 304.

  let open = $state(false);

  const unread = $derived(notifications.unread);
  const items = $derived(notifications.items);

  // Rendered rather than counted past this. A badge is a nudge, not a
  // measurement, and four digits in a 20px circle is neither.
  const badge = $derived(unread > 99 ? "99+" : String(unread));

  /**
   * Where a row goes when tapped.
   *
   * The two recaps are different screens reading different endpoints — your own
   * session and another lifter's are not one route with a parameter — which is
   * why the API hands over `sessionOwnerId` and leaves this decision to the
   * client. A `reply` is the case that makes it necessary: it reaches somebody
   * about a session that was never theirs.
   */
  function href(item: Notification): string | null {
    if (item.kind === "joined") return `/lifters/${item.actor.id}`;
    if (item.sessionId === undefined) return null;
    if (item.sessionOwnerId === auth.me?.id) {
      return `/sessions/${item.sessionId}/recap`;
    }
    if (item.sessionOwnerId === undefined) return null;
    return `/lifters/${item.sessionOwnerId}/sessions/${item.sessionId}/recap`;
  }

  /** The sentence a row reads as. The actor's name is drawn separately. */
  function summary(item: Notification): string {
    const workout = item.programDayName;
    switch (item.kind) {
      case "reaction":
        return workout ? `applauded your ${workout}` : "applauded your session";
      case "comment":
        return workout ? `commented on your ${workout}` : "commented on your session";
      case "reply":
        return workout ? `also replied on ${workout}` : "also replied on a session";
      case "joined":
        return "joined the gym";
    }
  }

  function name(item: Notification): string {
    return item.actor.displayName || item.actor.username;
  }

  function go(item: Notification) {
    const target = href(item);
    if (target === null) return;
    open = false;
    push(target);
  }

  // Opening is not reading. The two buttons are the whole point of storing
  // notifications rather than deriving them, and auto-reading on open would
  // collapse them back into one gesture — so a lifter who glances at the panel
  // mid-set can still come back to what was new. What opening DOES do is ask
  // for a fresh page, since the badge that drew them here may be up to a minute
  // old.
  $effect(() => {
    if (open) void poll();
  });

  const itemClass =
    "flex w-full cursor-pointer items-start gap-2 rounded-sm px-2 py-2 text-left text-sm text-foreground outline-none transition data-highlighted:bg-primary data-highlighted:text-primary-foreground";
</script>

<DropdownMenu.Root bind:open>
  <DropdownMenu.Trigger
    class="relative flex items-center rounded-full p-2 text-ink transition hover:bg-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
    aria-label={unread === 0
      ? "Notifications"
      : `Notifications, ${unread} unread`}
  >
    <Bell class="size-4" aria-hidden="true" />
    {#if unread > 0}
      <!-- aria-hidden because the count is already in the trigger's label
           above; announcing it twice is how a screen reader reads "5 5". -->
      <span
        class="absolute -right-0.5 -top-0.5 min-w-4 rounded-full bg-primary px-1 text-center text-[0.6rem] font-bold leading-4 text-primary-foreground tabular-nums"
        aria-hidden="true"
      >
        {badge}
      </span>
    {/if}
  </DropdownMenu.Trigger>

  <DropdownMenu.Portal>
    <!-- No backdrop-blur, like every other surface here: bg-card is opaque, so
         there is no backdrop left to see through it. -->
    <DropdownMenu.Content
      sideOffset={8}
      align="end"
      class="z-50 flex max-h-[26rem] w-80 flex-col rounded-md border border-border/60 bg-card shadow-lg shadow-black/40"
    >
      <div
        class="flex items-center justify-between gap-2 border-b border-border/60 px-3 py-2"
      >
        <span
          class="text-[0.65rem] font-semibold uppercase tracking-[0.25em] text-muted-foreground"
        >
          Notifications
        </span>
        <div class="flex items-center gap-1">
          <!-- Plain buttons rather than DropdownMenu.Item: an Item closes the
               menu when it is chosen, and both of these act ON the list the
               lifter is looking at. Marking read and watching the dots go is
               the feedback. -->
          <button
            type="button"
            class="rounded-sm px-2 py-1 text-xs text-muted-foreground transition hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-40"
            disabled={unread === 0}
            onclick={() => void markAllRead()}
          >
            Mark all read
          </button>
          <button
            type="button"
            class="rounded-sm px-2 py-1 text-xs text-muted-foreground transition hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-40"
            disabled={items.length === 0}
            onclick={() => void clearAll()}
          >
            Clear all
          </button>
        </div>
      </div>

      <div class="overflow-y-auto p-1">
        {#if items.length > 0}
          {#each items as item (item.id)}
            <DropdownMenu.Item
              class={itemClass}
              onSelect={() => go(item)}
              closeOnSelect={false}
            >
              <Avatar user={item.actor} size={28} />
              <span class="flex min-w-0 flex-1 flex-col gap-0.5">
                <span class="break-words">
                  <span class="font-semibold">{name(item)}</span>
                  {summary(item)}
                  {#if item.kind === "reaction" && item.emoji}
                    <span aria-hidden="true">{item.emoji}</span>
                  {/if}
                </span>
                {#if item.commentBody}
                  <span class="break-words text-xs text-muted-foreground">
                    “{item.commentBody}”
                  </span>
                {/if}
                <span class="text-xs text-muted-foreground">
                  {relativeTime(item.createdAt)}
                </span>
              </span>
              {#if !item.readAt}
                <!-- The unread mark. Decoration only: "unread" is already in
                     the trigger's accessible name as a count, and a dot per
                     row would announce as noise between every sentence. -->
                <span
                  class="mt-1.5 size-2 shrink-0 rounded-full bg-primary"
                  aria-hidden="true"
                ></span>
              {/if}
            </DropdownMenu.Item>
          {/each}
        {:else if notifications.failed}
          <p class="px-3 py-6 text-center text-sm text-destructive" role="alert">
            Couldn't load these.
          </p>
        {:else if notifications.loaded}
          <p class="px-3 py-6 text-center text-sm text-muted-foreground">
            Nothing yet. When somebody applauds or comments on your training,
            it'll show up here.
          </p>
        {:else}
          <!-- Only before the first poll has ever landed. After that the list
               is drawn from what is already held, so opening the panel never
               flashes this. -->
          <div class="h-20 animate-pulse" aria-hidden="true"></div>
        {/if}
      </div>
    </DropdownMenu.Content>
  </DropdownMenu.Portal>
</DropdownMenu.Root>
