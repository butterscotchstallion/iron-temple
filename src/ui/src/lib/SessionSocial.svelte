<script lang="ts">
  import { onMount, tick } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import Avatar from "./Avatar.svelte";
  import Loading from "./skeleton/Loading.svelte";
  import Skeleton from "./skeleton/Skeleton.svelte";
  import { auth } from "./auth.svelte";
  import { prefersReducedMotion } from "./reducedMotion";
  import { watchSession } from "./live.svelte";
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
  //
  // highlightCommentId is a deep link: a notification said "Ada commented on
  // your Workout A", and this is which comment she left. The recap routes read
  // it off the query string and hand it down rather than this component reading
  // the URL, because the two recap screens are different routes and the one
  // that knows which is which is the route.
  let {
    sessionId,
    ownerId,
    highlightCommentId = null,
  }: {
    sessionId: number;
    ownerId: number | null;
    highlightCommentId?: number | null;
  } = $props();

  // The emoji come from the generated client rather than a list written here, so
  // the buttons cannot offer one the server would reject.
  const EMOJI = Object.values(ReactionEmoji);

  let reactions = $state<SessionReaction[]>([]);
  let comments = $state<SessionComment[]>([]);
  let body = $state("");
  let posting = $state(false);
  let error = $state<string | null>(null);

  // How much of a conversation this card holds at once.
  //
  // The endpoint pages from the NEWEST end — offset counts back from the most
  // recent comment — so the first page is the tail of the thread, which is
  // what somebody opening a session wants and what a notification points at.
  // Paging walks upwards into the older part.
  const COMMENT_PAGE = 20;

  // Every comment on the session, not just the ones held here, so the control
  // below can say how many are above them.
  let commentTotal = $state(0);
  let loadingEarlier = $state(false);
  const earlierCount = $derived(Math.max(0, commentTotal - comments.length));

  // Which comment to mark when it lands. Cleared once it has been, so the
  // highlight is a one-off arrival cue and not a permanent decoration — and so
  // that posting a comment afterwards does not re-scroll the page.
  let marked = $state<number | null>(null);

  // The two actions that used to run silently.
  //
  // Posting a comment has always had `posting` behind it; applauding and
  // removing did not, so the tap that mattered most — a reaction, which costs
  // two round trips because the count is refetched rather than guessed — gave
  // no sign it had been heard. On anything slower than a LAN that reads as a
  // dead button, and the second tap is a withdrawal of the first.
  let pending = $state<ReactionEmoji | null>(null);
  let removingId = $state<number | null>(null);

  // Whether the first pair of requests has come back.
  //
  // This card used to draw immediately with both lists empty and then grow as
  // each landed — the reaction row gaining its counts, the comment list
  // unfolding from nothing — which pushed the comment box down the page a beat
  // after the lifter had reached for it. There was also nothing here to tell a
  // screen reader that anything was on its way.
  //
  // One flag for both requests rather than one each: they are fired together and
  // land within a few milliseconds of each other, and two flags would mean two
  // separate moments of the card changing height.
  let loaded = $state(false);

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
    const result = await listSessionComments(sessionId, { limit: COMMENT_PAGE });
    if (result.status === 200) {
      comments = result.data.items;
      commentTotal = result.data.total;
    }
  }

  /**
   * Re-read the conversation without losing what is already on screen.
   *
   * A live update must not silently drop the earlier pages somebody has walked
   * back through, so this asks for as much as is currently held rather than for
   * the first page. Bounded by the endpoint's own limit of 100: past that the
   * card falls back to the tail of the thread, which is where a reader who has
   * paged that far up is not looking anyway.
   */
  async function refreshComments() {
    const held = Math.min(Math.max(comments.length, COMMENT_PAGE), 100);
    const result = await listSessionComments(sessionId, { limit: held });
    if (result.status === 200) {
      comments = result.data.items;
      commentTotal = result.data.total;
    }
  }

  /**
   * Fetch the page above the one held, and put it on top.
   *
   * Prepended rather than appended: what comes back is OLDER than everything
   * already on screen, and the list reads downwards. Offset is the number of
   * comments already held, which is exactly how far back from the newest the
   * next page starts.
   *
   * Returns whether anything arrived, so the deep-link walk below can stop.
   */
  async function loadEarlier(): Promise<boolean> {
    if (loadingEarlier || earlierCount === 0) return false;
    loadingEarlier = true;
    const result = await listSessionComments(sessionId, {
      limit: COMMENT_PAGE,
      offset: comments.length,
    });
    loadingEarlier = false;

    if (result.status !== 200) {
      error = "Couldn't load the earlier comments.";
      return false;
    }
    commentTotal = result.data.total;
    if (result.data.items.length === 0) return false;
    comments = [...result.data.items, ...comments];
    return true;
  }

  /**
   * Bring the comment a notification pointed at into view, paging back to find
   * it if it is above the first page.
   *
   * Bounded rather than "until found": a thread long enough to need ten pages
   * means something else is wrong, and a loop that cannot terminate on a
   * missing id would spin. A comment that has since been deleted simply is not
   * there, and this gives up quietly — the lifter still gets the conversation.
   */
  async function revealMarked(id: number) {
    for (let page = 0; page < 10; page += 1) {
      if (comments.some((c) => c.id === id)) break;
      if (!(await loadEarlier())) break;
    }
    if (!comments.some((c) => c.id === id)) return;

    marked = id;
    // After the row exists in the DOM. tick() is Svelte's own "the update has
    // been applied", which is the only honest moment to look the node up.
    await tick();
    const node = document.getElementById(`comment-${id}`);
    if (!node) return;
    node.scrollIntoView({
      // Respected rather than assumed: this is a movement somebody did not ask
      // for, which is exactly the kind the preference is about.
      behavior: prefersReducedMotion() ? "auto" : "smooth",
      block: "center",
    });
  }

  async function toggle(emoji: ReactionEmoji) {
    if (isMine || pending !== null) return;
    error = null;
    pending = emoji;
    const pressed = minePressed(emoji);
    const result = pressed
      ? await removeSessionReaction(sessionId, { emoji })
      : await addSessionReaction(sessionId, { emoji });
    if (result.status !== 204) {
      pending = null;
      error = "Couldn't save that.";
      return;
    }
    // Refetched rather than adjusted in place. The server owns the count, other
    // lifters may have reacted since this loaded, and a toggle is one cheap round
    // trip — guessing the new number would be the only way this could show one
    // the server disagrees with.
    await loadReactions();
    // Cleared after the refetch, not after the write. The count is what the tap
    // is FOR, and releasing the button while it is still the old number invites
    // the second tap that undoes the first.
    pending = null;
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
      // The server names the reason — too long, blank, or posting faster than
      // the install allows — which is more use than a generic failure.
      error = result.data?.message ?? "Couldn't post that.";
      return;
    }
    // Appended rather than refetched: the list is oldest-first, so a new comment
    // belongs at the end, and the response is the row.
    comments = [...comments, result.data];
    commentTotal += 1;
    body = "";
  }

  async function remove(comment: SessionComment) {
    if (removingId !== null) return;
    error = null;
    removingId = comment.id;
    const result = await deleteSessionComment(sessionId, comment.id);
    removingId = null;
    if (result.status !== 204) {
      error = "Couldn't remove that.";
      return;
    }
    comments = comments.filter((c) => c.id !== comment.id);
    // The total counts the thread, so removing a row from the page has to take
    // it off the count as well — otherwise "1 earlier comment" appears for one
    // that was just deleted.
    commentTotal = Math.max(0, commentTotal - 1);
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

  // Applause and conversation arriving while this card is on screen.
  //
  // Before this, two lifters talking at once each saw a stale list until one of
  // them reloaded — the recap is the screen where a conversation actually
  // happens and it was the one screen that never refetched.
  //
  // SPLIT BY KIND so a run of reactions does not refetch the comment list and
  // vice versa; `resync` arrives on every (re)connect and refetches both,
  // because a gap in the connection is a gap in both.
  //
  // The effect's teardown is watchSession's, which is refcounted — so this card
  // appearing twice on one screen, or being replaced when the route changes,
  // does not leave the other one unsubscribed.
  $effect(() =>
    watchSession(sessionId, (kind) => {
      if (kind !== "comment") void loadReactions();
      if (kind !== "reaction") void refreshComments();
    }),
  );

  onMount(() => {
    // Both at once: they are separate requests and neither waits on the other.
    // The card stays in its placeholder until both have answered, so it changes
    // height once rather than twice. A failure still clears it — the emoji
    // buttons are usable with no counts behind them, and a permanent shimmer
    // would be a worse lie than a zero.
    void Promise.all([loadReactions(), loadComments()])
      .finally(() => {
        loaded = true;
      })
      .then(() => {
        // After `loaded`, deliberately. The comment rows do not exist in the
        // DOM until the placeholder is replaced, so there is nothing to scroll
        // to before this point.
        if (highlightCommentId !== null) return revealMarked(highlightCommentId);
      });
  });
</script>

<Card class="flex flex-col gap-4 p-4" data-testid="session-social">
  {#if !loaded}
    <!-- The reaction row only, at the exact height its pills will be. The
         comment list above the box is NOT reserved: most sessions have no
         comments, so holding a couple of rows for them would collapse when the
         answer came back — the same reflow, pointed the other way. -->
    <Loading label="Loading applause and comments" class="flex flex-row flex-wrap items-center gap-2">
      {#each EMOJI as emoji (emoji)}
        <Skeleton class="h-[34px] w-14 rounded-full" />
      {/each}
    </Loading>
  {:else}
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
          {@const saving = pending === emoji}
          <!-- The whole row goes inert while one of them is in flight, because
               the count they all read from is about to be replaced wholesale by
               the refetch. `disabled` alone would be silent to a screen reader
               mid-press, so aria-busy says which one is working. -->
          <button
            type="button"
            onclick={() => toggle(emoji)}
            aria-pressed={minePressed(emoji)}
            aria-label={saving ? `Saving ${emoji}` : `React with ${emoji}`}
            aria-busy={saving}
            disabled={pending !== null}
            class="inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-sm transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-60 {minePressed(
              emoji,
            )
              ? 'border-primary bg-primary/15'
              : 'border-border/60 hover:bg-white/5'}"
          >
            <span
              class={saving ? "animate-pulse motion-reduce:animate-none" : ""}
              aria-hidden="true"
            >
              {emoji}
            </span>
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
    {#if earlierCount > 0}
      <!-- Only offered when there is something above the page. The count is of
           the whole thread, which is why the endpoint returns a total: a short
           page cannot say how much it left behind. -->
      <button
        type="button"
        onclick={() => void loadEarlier()}
        disabled={loadingEarlier}
        aria-busy={loadingEarlier}
        class="self-start rounded-md px-1 py-0.5 text-xs font-semibold text-primary transition hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-60"
      >
        {loadingEarlier
          ? "Loading…"
          : `Show ${earlierCount} earlier comment${earlierCount === 1 ? "" : "s"}`}
      </button>
    {/if}

    {#if comments.length > 0}
      <ul class="flex flex-col divide-y divide-border/60">
        {#each comments as comment (comment.id)}
          <li
            id={`comment-${comment.id}`}
            class="flex items-start gap-2.5 py-2.5 {marked === comment.id
              ? 'animate-none rounded-md bg-primary/10 ring-1 ring-primary/40'
              : ''}"
          >
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
              {@const removing = removingId === comment.id}
              <button
                type="button"
                onclick={() => remove(comment)}
                aria-label="Remove this comment"
                aria-busy={removing}
                disabled={removingId !== null}
                class="shrink-0 rounded-md p-1 text-muted-foreground transition hover:bg-white/5 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-40"
              >
                <Trash2
                  class="size-4 {removing ? 'animate-pulse motion-reduce:animate-none' : ''}"
                  aria-hidden="true"
                />
              </button>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  {/if}

  <!-- Outside the branch above, deliberately. Posting a comment needs nothing
       either request returns, so there is no reason to withhold the box — and
       leaving it mounted means it does not move when they land. -->
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
