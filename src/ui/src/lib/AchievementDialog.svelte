<script lang="ts">
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { Button } from "$lib/components/ui/button";
  import Crown from "@lucide/svelte/icons/crown";
  import Avatar from "./Avatar.svelte";
  import LifterName from "./LifterName.svelte";
  import Loading from "./skeleton/Loading.svelte";
  import Skeleton from "./skeleton/Skeleton.svelte";
  import { achievementBySlug, holdersOf } from "./achievements.svelte";
  import { relativeTime } from "./date";
  import type { Notification } from "./api";

  // One crown at a time, out of a notification row that folded several.
  //
  // WHY THIS IS A DIALOG AND NOT A PAGE
  //
  // The panel folds every crown on the install into a single row, because a crown
  // has no session and rows group by (kind, session). That row can only say
  // "Grace and 2 others took crowns" — naming one board while folding four would
  // be a claim the group does not support. So the detail has to go somewhere, and
  // a dialog is where: the reader is mid-panel, the answer is three short cards,
  // and sending them to a route would lose the panel they were reading.
  //
  // AlertDialog rather than a plain dialog because it is the only modal primitive
  // vendored here, and every dialog in this app is one — including the purely
  // informational StreakCard and ShareCardDialog. Consistency beats the semantic
  // quibble about `role="alertdialog"`.

  let {
    open = $bindable(false),
    items,
    loading = false,
    onLeaderboard,
  }: {
    open?: boolean;
    /** The row's members, newest first — the order the API returns them in. */
    items: Notification[];
    /** While the members are still on their way. */
    loading?: boolean;
    /** Take the reader to the standings. The caller closes whatever it needs to. */
    onLeaderboard: () => void;
  } = $props();

  // Which card is showing.
  //
  // Reset whenever the LIST changes rather than when `open` does, and the
  // difference matters: the members arrive after the dialog opens, so keying off
  // `open` would reset the cursor to 0 on a list that had just been replaced and
  // then leave it there — which is the same result by luck, until a second row is
  // opened without closing the first and the cursor survives into a shorter list.
  // Clamping on the list is what makes an out-of-range cursor impossible.
  let cursor = $state(0);
  $effect(() => {
    void items;
    cursor = 0;
  });

  const total = $derived(items.length);
  const current = $derived<Notification | null>(items[cursor] ?? null);
  const hasNext = $derived(cursor < total - 1);

  // The catalogue entry behind this card, for the label and the description.
  // Null until /achievements has landed, which is why every use of it has a
  // fallback — the dialog must read sensibly on a cold client.
  const achievement = $derived(achievementBySlug(current?.achievementSlug));

  const title = $derived(achievement?.label ?? "A crown was taken");

  /**
   * "Ada and Bea", "Ada, Bea and Cara" — a list that reads as a sentence.
   *
   * Ties on a board can run to three or more, and joining every name with "and"
   * gives "Ada and Bea and Cara". Same shape `subject()` builds in the panel.
   */
  function nameList(names: string[]): string {
    if (names.length <= 1) return names.join("");
    return `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
  }

  /**
   * Whether the lifter who took this crown still has it, and who has it if not.
   *
   * A notification is an event and can be hours old, so this is the line that
   * stops the dialog reporting stale news as current. Free: the client already
   * holds who is wearing what, so no request is needed to answer it.
   *
   * A tagged object rather than a string with two magic values, because the magic
   * values were "still" and "nobody" and a lifter is allowed to call themselves
   * either of those. Null when the catalogue has not loaded — saying nothing beats
   * saying "nobody holds this" on the strength of a list we have not fetched.
   */
  type Standing = { held: "still" } | { held: "nobody" } | { held: "moved"; by: string };

  const standing = $derived.by<Standing | null>(() => {
    if (!current?.achievementSlug || achievement === null) return null;
    const holders = holdersOf(current.achievementSlug);
    if (holders.some((h) => h.id === current.actor.id)) return { held: "still" };
    if (holders.length === 0) return { held: "nobody" };
    return {
      held: "moved",
      by: nameList(holders.map((h) => h.displayName || h.username)),
    };
  });
</script>

<AlertDialog.Root bind:open>
  <AlertDialog.Content class="sm:max-w-md" data-testid="achievement-dialog">
    <AlertDialog.Header>
      <AlertDialog.Media>
        <Crown class="text-primary" aria-hidden="true" />
      </AlertDialog.Media>
      <AlertDialog.Title>
        {title}
        {#if total > 1}
          <!-- The position, so a reader knows how many are left before they
               start pressing Next. Inside the title so it is part of the
               dialog's accessible name rather than a number floating beside
               it. -->
          <span class="ml-1 text-sm font-normal text-muted-foreground">
            {cursor + 1} of {total}
          </span>
        {/if}
      </AlertDialog.Title>
      <AlertDialog.Description>
        {achievement?.description ?? "Somebody took the top of a leaderboard board."}
      </AlertDialog.Description>
    </AlertDialog.Header>

    <!-- aria-live, because Next replaces this block in place. Without it a
         screen reader is told the dialog is open and then nothing more, however
         many times the reader advances. Same call RestTimer and Toaster make. -->
    <div aria-live="polite">
      {#if loading}
        <Loading label="Loading the achievement">
          <div class="flex items-center gap-3">
            <Skeleton class="size-8 shrink-0 rounded-full" />
            <Skeleton text="sm" class="w-40" />
          </div>
        </Loading>
      {:else if current}
        <div class="flex items-center gap-3">
          <Avatar user={current.actor} size={32} />
          <p class="min-w-0 text-sm text-foreground">
            <!-- crowns={false}: this whole dialog is about one crown, named in
                 the heading. Decorating the name too would repeat it, and on a
                 lifter leading three boards would add two the card is not
                 talking about. -->
            <LifterName lifter={current.actor} class="font-semibold" crowns={false} />
            <span class="text-muted-foreground">
              took this {relativeTime(current.createdAt)}
            </span>
          </p>
        </div>

        {#if standing !== null}
          <p class="mt-3 text-xs text-muted-foreground">
            {#if standing.held === "still"}
              Still theirs.
            {:else if standing.held === "nobody"}
              Nobody holds this right now.
            {:else}
              Since taken by {standing.by}.
            {/if}
          </p>
        {/if}
      {:else}
        <!-- The members resolved to nothing. Only reachable if the row was
             cleared or archived between the tap and the response, which is rare
             and not an error worth a banner — the dialog just has nothing to
             say. -->
        <p class="text-sm text-muted-foreground">
          This one isn't here any more.
        </p>
      {/if}
    </div>

    <AlertDialog.Footer>
      <!-- Plain Buttons rather than AlertDialog.Action: Action closes the dialog
           on click, which would make Next dismiss the thing it was meant to
           advance. ShareCardDialog and UpdatePrompt both carry the same warning
           for the same reason. Cancel is the one that SHOULD close. -->
      <Button variant="outline" onclick={onLeaderboard}>Leaderboard</Button>
      {#if hasNext}
        <Button onclick={() => (cursor += 1)}>Next</Button>
      {/if}
      <AlertDialog.Cancel>Close</AlertDialog.Cancel>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
