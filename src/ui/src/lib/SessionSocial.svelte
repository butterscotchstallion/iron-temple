<script lang="ts">
  import { onMount } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import Avatar from "./Avatar.svelte";
  import { auth } from "./auth.svelte";
  import {
    addSessionComment,
    addSessionReaction,
    deleteSessionComment,
    listSessionComments,
    listSessionReactions,
    removeSessionReaction,
    ReactionEmoji,
    type SessionComment,
    type SessionReaction,
  } from "./api";

  // Applause and conversation under a session recap.
  //
  // NOT IN THE OFFLINE WRITE QUEUE, and must not be added to it. That queue exists
  // because a rep tapped at a rack in a basement has to survive having no network
  // — see writeQueue.svelte.ts. Reacting to somebody's workout is a couch
  // activity: it happens where there is signal, nothing is lost if it fails, and a
  // reaction replayed twenty minutes later is a reaction rather than a rescued
  // set. So these are plain calls, and a failure is reported and forgotten.
  //
  // Mounted below the recap rather than folded into it, because the recap is built
  // to be answerable from memory with a dead network and this is not. If the
  // requests below fail the recap above them is unaffected.
  let { sessionId, ownerId }: { sessionId: number; ownerId: number | null } = $props();

  // The emoji come from the generated client rather than a list written here, so
  // the buttons cannot offer one the server would reject.
  const EMOJI = Object.values(ReactionEmoji);

  let reactions = $state<SessionReaction[]>([]);
  let comments = $state<SessionComment[]>([]);
  let body = $state("");
  let posting = $state(false);
  let error = $state<string | null>(null);

  // Applauding your own workout is refused by the API, so the buttons are not
  // offered — a control that always errors is worse than no control. The counts
  // still show: seeing who applauded your session is the point of having it.
  const isMine = $derived(ownerId !== null && ownerId === auth.me?.id);

  // Typed as ReactionEmoji rather than string throughout, so the only values that
  // can reach the API are ones the contract lists.
  function countFor(emoji: ReactionEmoji): number {
    return reactions.find((r) => r.emoji === emoji)?.count ?? 0;
  }
  function minePressed(emoji: ReactionEmoji): boolean {
    return reactions.find((r) => r.emoji === emoji)?.mine ?? false;
  }

  async function loadReactions() {
    const result = await listSessionReactions(sessionId);
    if (result.status === 200) reactions = result.data;
  }
  async function loadComments() {
    const result = await listSessionComments(sessionId);
    if (result.status === 200) comments = result.data;
  }

  async function toggle(emoji: ReactionEmoji) {
    if (isMine) return;
    error = null;
    const pressed = minePressed(emoji);
    const result = pressed
      ? await removeSessionReaction(sessionId, { emoji })
      : await addSessionReaction(sessionId, { emoji });
    if (result.status !== 204) {
      error = "Couldn't save that.";
      return;
    }
    // Refetched rather than adjusted in place. The server owns the count, other
    // lifters may have reacted since this loaded, and a toggle is one cheap round
    // trip — guessing the new number would be the only way this could show one
    // the server disagrees with.
    await loadReactions();
  }

  async function post(event: SubmitEvent) {
    event.preventDefault();
    const trimmed = body.trim();
    if (posting || trimmed === "") return;

    posting = true;
    error = null;
    const result = await addSessionComment(sessionId, { body: trimmed });
    posting = false;

    if (result.status !== 201) {
      // The server names the reason — too long, blank — which is more use than a
      // generic failure.
      error = result.data?.message ?? "Couldn't post that.";
      return;
    }
    // Appended rather than refetched: the list is oldest-first, so a new comment
    // belongs at the end, and the response is the row.
    comments = [...comments, result.data];
    body = "";
  }

  async function remove(comment: SessionComment) {
    error = null;
    const result = await deleteSessionComment(sessionId, comment.id);
    if (result.status !== 204) {
      error = "Couldn't remove that.";
      return;
    }
    comments = comments.filter((c) => c.id !== comment.id);
  }

  // The author may remove their own; the install's owner may remove any. Mirrors
  // the API rule — it is not the check that matters, which is the server's.
  function canRemove(comment: SessionComment): boolean {
    return comment.author.id === auth.me?.id || auth.me?.isAdmin === true;
  }

  function saidAt(iso: string): string {
    return new Date(iso).toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
    });
  }

  onMount(() => {
    // Both at once: they are separate requests and neither waits on the other.
    void loadReactions();
    void loadComments();
  });
</script>

<Card class="flex flex-col gap-4 p-4" data-testid="session-social">
  <!-- Reactions -->
  <div class="flex flex-wrap items-center gap-2">
    {#if isMine}
      <!-- Read-only: the counts, without controls that would only ever 403. -->
      {#each reactions as reaction (reaction.emoji)}
        <span
          class="inline-flex items-center gap-1 rounded-full border border-border/60 px-2.5 py-1 text-sm"
        >
          <span aria-hidden="true">{reaction.emoji}</span>
          <span class="text-xs font-semibold text-muted-foreground">{reaction.count}</span>
        </span>
      {/each}
      {#if reactions.length === 0}
        <span class="text-xs text-muted-foreground">No reactions yet.</span>
      {/if}
    {:else}
      {#each EMOJI as emoji (emoji)}
        {@const count = countFor(emoji)}
        <button
          type="button"
          onclick={() => toggle(emoji)}
          aria-pressed={minePressed(emoji)}
          aria-label={`React with ${emoji}`}
          class="inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-sm transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary {minePressed(
            emoji,
          )
            ? 'border-primary bg-primary/15'
            : 'border-border/60 hover:bg-white/5'}"
        >
          <span aria-hidden="true">{emoji}</span>
          {#if count > 0}
            <span class="text-xs font-semibold text-muted-foreground">{count}</span>
          {/if}
        </button>
      {/each}
    {/if}
  </div>

  {#if error}
    <p class="text-sm text-destructive" role="status">{error}</p>
  {/if}

  <!-- Comments -->
  {#if comments.length > 0}
    <ul class="flex flex-col divide-y divide-border/60">
      {#each comments as comment (comment.id)}
        <li class="flex items-start gap-2.5 py-2.5">
          <Avatar user={comment.author} size={28} />
          <div class="min-w-0 flex-1">
            <p class="flex items-baseline gap-2">
              <span class="truncate text-sm font-semibold text-foreground">
                {comment.author.displayName || comment.author.username}
              </span>
              <span class="shrink-0 text-xs text-muted-foreground">
                {saidAt(comment.createdAt)}
              </span>
            </p>
            <!-- break-words, not truncate: a comment is the content, and 256
                 characters wrap rather than being cut off. -->
            <p class="break-words text-sm text-foreground/90">{comment.body}</p>
          </div>
          {#if canRemove(comment)}
            <button
              type="button"
              onclick={() => remove(comment)}
              aria-label="Remove this comment"
              class="shrink-0 rounded-md p-1 text-muted-foreground transition hover:bg-white/5 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
              <Trash2 class="size-4" aria-hidden="true" />
            </button>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  <form class="flex items-center gap-2" onsubmit={post}>
    <label class="sr-only" for={`comment-${sessionId}`}>Add a comment</label>
    <input
      bind:value={body}
      id={`comment-${sessionId}`}
      name="body"
      autocomplete="off"
      maxlength="256"
      placeholder="Say something"
      class="min-w-0 flex-1 rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
    />
    <Button type="submit" size="sm" disabled={posting || body.trim() === ""}>
      {posting ? "…" : "Post"}
    </Button>
  </form>
</Card>
