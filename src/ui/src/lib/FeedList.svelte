<script lang="ts">
  import { link } from "svelte-spa-router";
  import Heart from "@lucide/svelte/icons/heart";
  import MessageCircle from "@lucide/svelte/icons/message-circle";
  import Avatar from "./Avatar.svelte";
  import { formatLongDate } from "./date";
  import { formatVolume } from "./volume";
  import type { FeedEntry } from "./api";

  // The rows of the feed, without the card around them or the loading around
  // that.
  //
  // Split out because two surfaces draw the same rows: a short card at the foot
  // of Home, and the /feed page that pages through all of them. They differ in
  // how much they fetch and what they say when there is nothing, which is why
  // this takes items rather than fetching its own.
  let { items }: { items: FeedEntry[] } = $props();

  // Each row links to the recap under its OWNER's id, not the reader's. That is
  // the endpoint's contract — the session is scoped by the lifter in the path —
  // so a link built from the reader's id would 404 on every row.
  function recapHref(entry: FeedEntry): string {
    return `/lifters/${entry.lifter.id}/sessions/${entry.id}/recap`;
  }
</script>

<ul class="flex flex-col divide-y divide-border/60">
  {#each items as entry (entry.id)}
    <li>
      <a
        href={recapHref(entry)}
        use:link
        class="-mx-2 flex items-center gap-3 rounded-md px-2 py-3 transition hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      >
        <Avatar user={entry.lifter} size={36} />

        <span class="flex min-w-0 flex-1 flex-col">
          <span class="truncate text-sm font-semibold text-foreground">
            {entry.lifter.displayName || entry.lifter.username}
          </span>
          <span class="truncate text-xs text-muted-foreground">
            {entry.programDayName} · {formatLongDate(entry.performedOn)}
          </span>
          <!-- Counts only, and only when there are any. The row is a link to the
               recap, and a reaction button nested inside a link is a click target
               fighting its own parent — giving applause needs the session in front
               of you, which is where the row goes. -->
          {#if entry.reactionCount > 0 || entry.commentCount > 0}
            <span class="mt-0.5 flex items-center gap-2.5 text-xs text-muted-foreground">
              {#if entry.reactionCount > 0}
                <span class="inline-flex items-center gap-1">
                  <Heart class="size-3" aria-hidden="true" />
                  <span>{entry.reactionCount}</span>
                  <span class="sr-only">
                    {entry.reactionCount === 1 ? "reaction" : "reactions"}
                  </span>
                </span>
              {/if}
              {#if entry.commentCount > 0}
                <span class="inline-flex items-center gap-1">
                  <MessageCircle class="size-3" aria-hidden="true" />
                  <span>{entry.commentCount}</span>
                  <span class="sr-only">
                    {entry.commentCount === 1 ? "comment" : "comments"}
                  </span>
                </span>
              {/if}
            </span>
          {/if}
        </span>

        <span class="flex shrink-0 flex-col items-end">
          <span class="text-sm font-bold text-foreground">
            {formatVolume(entry.volumeLb)}
          </span>
          <span class="text-xs text-muted-foreground">
            {entry.setCount === 1 ? "1 set" : `${entry.setCount} sets`}
          </span>
        </span>
      </a>
    </li>
  {/each}
</ul>
