<script lang="ts">
  import { DropdownMenu } from "bits-ui";
  import { push } from "svelte-spa-router";
  import Bell from "@lucide/svelte/icons/bell";
  import Avatar from "./Avatar.svelte";
  import Loading from "./skeleton/Loading.svelte";
  import SkeletonRows from "./skeleton/SkeletonRows.svelte";
  import { auth } from "./auth.svelte";
  import { achievementLabel } from "./achievements.svelte";
  import { houseById } from "./houses.svelte";
  import Timestamp from "./Timestamp.svelte";
  import {
    clearAll,
    markAllRead,
    markRead,
    notifications,
    poll,
  } from "./notifications.svelte";
  import AchievementDialog from "./AchievementDialog.svelte";
  import { getNotificationGroupMembers, type Notification } from "./api";

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
  //
  // EVERY ROW IS A GROUP. The API folds everything the lifter was told about one
  // session into a single item — see listNotifications in openapi.yaml — so a row
  // says "Bob, Cara and 4 others applauded your Push Day" rather than being one
  // of six rows that each say it once. Three things follow, and they are the only
  // places this component knows about it: the subject is a phrase rather than a
  // name, the badge counts rows because the API counts them the same way, and
  // tapping a row marks everything it folded read.

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
    // A crown goes nowhere: it opens a dialog instead, because the row folds
    // every crown on the install and a route could only ever show one of them.
    // The leaderboard is still one press away, from inside that dialog.
    if (item.kind === "crown") return null;
    // A level row goes to the profile, which is where the rung is listed, and
    // deliberately NOT to the crown's dialog. That dialog is built around a
    // standing — "still theirs", "since taken by X", and a Leaderboard button —
    // and every one of those is meaningless for something that is never lost. The
    // profile is also where the ladder reads as a ladder, with the rungs still to
    // come under it.
    if (item.kind === "level") return `/lifters/${item.actor.id}`;
    // The three House rows go to the House, which is where every one of them can
    // be acted on: an owner approves from its page and a requester reads about it
    // there. The id is withheld on a row that folded more than one House — see
    // Notification.houseId — and such a row goes nowhere rather than picking one.
    if (
      item.kind === "house-request" ||
      item.kind === "house-approved" ||
      item.kind === "house-declined"
    ) {
      return item.houseId === undefined ? null : `/houses/${item.houseId}`;
    }
    if (item.sessionId === undefined) return null;

    // Which comment, appended so the recap can scroll to the sentence rather
    // than dropping the lifter at the top of a long page with the conversation
    // as its last card. A reaction has none and gets a bare recap link.
    const anchor = item.commentId === undefined ? "" : `?comment=${item.commentId}`;

    if (item.sessionOwnerId === auth.me?.id) {
      return `/sessions/${item.sessionId}/recap${anchor}`;
    }
    if (item.sessionOwnerId === undefined) return null;
    return `/lifters/${item.sessionOwnerId}/sessions/${item.sessionId}/recap${anchor}`;
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
      case "crown": {
        // Which board, when the API was willing to say. It withholds the slug
        // on a row that folded crowns from several boards — see the field's
        // note — and the catalogue can also simply not have loaded yet, so both
        // fall through to the unnamed sentence rather than to a blank.
        const board = achievementLabel(item.achievementSlug);
        return board ? `took the crown on ${board}` : "took a crown";
      }
      case "level": {
        // The rung's own name — "Journeyman", not "Level 10" — because the
        // catalogue holds the label and this install's owner may have reworded it.
        // Same two fallbacks as the crown, for the same two reasons: a folded row
        // of several rungs withholds the slug, and the catalogue may not have
        // landed.
        const rung = achievementLabel(item.achievementSlug);
        return rung ? `reached ${rung}` : "reached a new level";
      }
      // The House's name when the API named one and the list has loaded, and the
      // unnamed sentence otherwise — the same two fallbacks the crown needs, for
      // the same two reasons. A House can also have been deleted since, which is
      // why houseById can answer null for an id that was real.
      case "house-request": {
        const house = houseById(item.houseId);
        return house ? `asked to join ${house.name}` : "asked to join your House";
      }
      case "house-approved": {
        const house = houseById(item.houseId);
        return house ? `let you into ${house.name}` : "let you into their House";
      }
      case "house-declined": {
        const house = houseById(item.houseId);
        return house ? `turned down your request to join ${house.name}` : "turned down your request";
      }
    }
  }

  /**
   * What to call the row's representative actor.
   *
   * "You" when it is the caller, which only `crown` can reach: it is the one kind
   * whose recipient may be its own actor, because your own achievement is the thing
   * you most want a record of. Every other kind filters the actor out of the
   * recipients, so this branch is unreachable for them.
   *
   * Known limitation, stated rather than hidden: in a group that folds YOUR crown
   * together with a followed lifter's, your display name can still appear among
   * `otherActorNames` — that field carries names and no ids, so there is nothing
   * here to compare. Fixing it properly means widening the wire, and the common
   * case is a group of one.
   */
  function name(item: Notification): string {
    if (item.actor.id === auth.me?.id) return "You";
    return item.actor.displayName || item.actor.username;
  }

  /**
   * How many people a row names before it starts counting them instead.
   *
   * Two, which is where the sentence stops being a sentence: "Bob, Cara and Dan
   * and 3 others applauded your Push Day" is a list with a number bolted on.
   * The API sends up to two spare names so this can be raised without touching
   * the contract.
   */
  const NAMED_ACTORS = 2;

  /**
   * Who a row is about, as one phrase — "Bob", "Bob and Cara", "Bob, Cara and 4
   * others".
   *
   * A row is a GROUP: everything the lifter was told about one session folds
   * into it, so the subject is plural as soon as two people applauded the same
   * training. `actor` is the most recent of them and the one whose avatar the
   * row draws, so it always leads.
   *
   * Built as a string rather than as markup so the whole subject can be bold as
   * one span and the pluralisation is one testable function. Bolding each name
   * separately would mean "and 4 others" either joining the emphasis for no
   * reason or breaking it mid-phrase.
   *
   * Degrades sensibly if `otherActorNames` is shorter than `actorCount` implies
   * — which is the ordinary case once a group outgrows the two names the API
   * sends: the remainder is simply counted.
   */
  function subject(item: Notification): string {
    const named = [name(item), ...(item.otherActorNames ?? [])].slice(
      0,
      NAMED_ACTORS,
    );
    const rest = item.actorCount - named.length;

    if (rest > 0) {
      return `${named.join(", ")} and ${rest} ${rest === 1 ? "other" : "others"}`;
    }
    // "Bob and Cara", never "Bob, Cara" — a two-item list takes "and".
    return named.join(" and ");
  }

  function go(item: Notification) {
    if (expandable(item)) {
      void expand(item);
      return;
    }
    const target = href(item);
    if (target === null) return;
    // Reading THIS one, which is what following it means — and since a row is a
    // group, that is everything it folded. Deliberately not awaited: the
    // navigation is the response to the tap, and the badge is already down
    // optimistically.
    void markRead(item.id);
    open = false;
    push(target);
  }

  /**
   * Whether a row leads anywhere.
   *
   * Almost all of them do. The exception is a `reply` about a session with no
   * owner — one predating accounts and never adopted — which has no recap route
   * to build. Such a row used to look identical to a live one and simply do
   * nothing when tapped, which reads as a broken panel rather than as a row
   * that is only a statement.
   */
  function navigable(item: Notification): boolean {
    return href(item) !== null;
  }

  /**
   * Whether a row opens a dialog rather than going somewhere.
   *
   * Only `crown`, and still only `crown` now that a second kind of achievement
   * exists. It is not "achievements open a dialog": it is that a crown has a
   * STANDING to show and nowhere to send anybody — the row folds every crown on
   * the install and no single route could show them all. A level rung has a
   * profile to go to and no standing to report, so it navigates like `joined`.
   */
  function expandable(item: Notification): boolean {
    return item.kind === "crown";
  }

  /**
   * Whether a row does ANYTHING when tapped.
   *
   * Split from `navigable` when crowns stopped navigating. The two were the same
   * question until then, and conflating them would have made the crown row inert:
   * `disabled` keys off this, and a row that leads nowhere because it opens a
   * dialog is not a row that does nothing.
   */
  function interactive(item: Notification): boolean {
    return navigable(item) || expandable(item);
  }

  // ---- the achievement dialog ----

  let detailOpen = $state(false);
  let detail = $state<Notification[]>([]);
  let detailLoading = $state(false);

  /**
   * Suppress the menu's focus-return, for the dialog path only.
   *
   * DropdownMenu.Content hands focus back to the bell when it closes. Closing the
   * panel and opening a dialog in the same tick can land that AFTER the dialog has
   * autofocused, leaving an open dialog with focus outside it — which traps a
   * keyboard reader behind a modal they cannot reach. Flagged rather than always
   * prevented, so the rows that navigate keep returning focus to the bell as they
   * always have.
   */
  let keepFocus = $state(false);

  /**
   * Open the dialog on one folded row.
   *
   * Marks the group read, because opening this IS reading it — the same call
   * following a row makes, and for the same reason. The panel closes first so
   * there is one layer rather than two stacked scroll locks.
   *
   * The dialog is opened BEFORE the members land, with `loading` set: the tap has
   * to be answered immediately, and a modal that appears a request later reads as
   * a dead row. A failure leaves it open with nothing in it, which the dialog says
   * plainly rather than raising a banner over a panel somebody may be reading
   * mid-set.
   */
  async function expand(item: Notification) {
    void markRead(item.id);
    keepFocus = true;
    open = false;
    detail = [];
    detailLoading = true;
    detailOpen = true;

    const result = await getNotificationGroupMembers(item.id);
    detailLoading = false;
    if (result.status === 200) detail = result.data.items;
    keepFocus = false;
  }

  function toLeaderboard() {
    detailOpen = false;
    push("/leaderboard");
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

  // The same row without the affordances: no pointer, no highlight on hover or
  // arrow-key focus. It still draws in full, because what it SAYS is worth
  // reading even though there is nowhere to go.
  const inertItemClass =
    "flex w-full items-start gap-2 rounded-sm px-2 py-2 text-left text-sm text-foreground outline-none";
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
      onCloseAutoFocus={(e) => {
        // Only when a dialog is taking over — see keepFocus. Returning focus to
        // the bell is right for every other way this closes.
        if (keepFocus) e.preventDefault();
      }}
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
            {@const acts = interactive(item)}
            <DropdownMenu.Item
              class={acts ? itemClass : inertItemClass}
              disabled={!acts}
              onSelect={() => go(item)}
              closeOnSelect={false}
            >
              <Avatar user={item.actor} size={28} />
              <span class="flex min-w-0 flex-1 flex-col gap-0.5">
                <span class="break-words">
                  <span class="font-semibold">{subject(item)}</span>
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
                <Timestamp
                  value={item.createdAt}
                  class="text-xs text-muted-foreground"
                />
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
               flashes this. Rows rather than one grey block, because that is
               what lands here — and because the shimmer now announces itself to
               a screen reader instead of leaving the panel silently empty. -->
          <!-- Two rows, which is also about the height of the "nothing yet"
               message that replaces them on an install where nobody has
               applauded anything. -->
          <Loading label="Loading your notifications" class="p-1">
            <SkeletonRows rows={2} avatar="size-7" rowClass="py-2" />
          </Loading>
        {/if}
      </div>
    </DropdownMenu.Content>
  </DropdownMenu.Portal>
</DropdownMenu.Root>

<!-- OUTSIDE the menu, deliberately. DropdownMenu.Content is presence-gated, so
     everything inside it is unmounted the moment the panel closes — and this
     dialog is opened BY closing the panel. Nested, it would be destroyed on the
     same tick it appeared. ExerciseCard.svelte carries the same warning for the
     same reason. -->
<AchievementDialog
  bind:open={detailOpen}
  items={detail}
  loading={detailLoading}
  onLeaderboard={toLeaderboard}
/>
