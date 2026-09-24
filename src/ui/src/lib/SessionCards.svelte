<script lang="ts">
  import { link } from "svelte-spa-router";
  import { Card } from "$lib/components/ui/card";
  import { formatLongDate } from "./date";
  import { formatVolume } from "./volume";
  import type { SessionSummary } from "./api";

  // A list of performed sessions, drawn the same way wherever it appears.
  //
  // Extracted from History when the profile grew a history of its own. The two
  // surfaces show the same rows from the same shape — the endpoints are
  // `GET /sessions` and `GET /lifters/{id}/sessions`, which return the identical
  // SessionList — so drawing them twice would be two places for "what a session
  // looks like" to drift.
  //
  // WHOSE HISTORY IS THE ONLY DIFFERENCE, and it is entirely about where a row
  // LINKS. `lifterId` null means the caller's own, which is the case that can
  // offer the live session screen; anything else is somebody else's, where that
  // screen would 404 — /sessions/{id} is scoped to the caller, so a reader-scoped
  // link to another lifter's session names a row the server will not hand over.
  // Their rows go straight to the recap, which is the cross-account read that
  // exists.
  let {
    sessions,
    lifterId = null,
  }: { sessions: SessionSummary[]; lifterId?: number | null } = $props();

  const mine = $derived(lifterId === null);

  /**
   * Where the card itself goes.
   *
   * On your own history that is the session — the thumb-sized target for
   * "open the workout I did" — and the recap is the smaller, deliberate tap in
   * the footer. On somebody else's there is only one destination, so the whole
   * card is it.
   */
  function cardHref(session: SessionSummary): string {
    if (mine) return `/sessions/${session.id}`;
    return `/lifters/${lifterId}/sessions/${session.id}/recap`;
  }

  /**
   * Whether the footer is worth drawing at all.
   *
   * A workout still in progress has no story yet, and offering one would invite
   * the lifter to leave the screen they are training on.
   */
  function hasFooter(session: SessionSummary): boolean {
    return session.volumeLb > 0 || session.isOver;
  }
</script>

<ul class="flex flex-col gap-3">
  {#each sessions as session (session.id)}
    <li>
      <Card class="p-4">
        <a use:link href={cardHref(session)} class="group block">
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="font-bold text-card-foreground">{session.programName}</p>
              <p class="mt-0.5 text-sm text-muted-foreground">
                {session.programDayName} · {formatLongDate(session.performedOn)}
              </p>
            </div>
            <p class="text-sm tabular-nums text-muted-foreground">
              {session.completedSetCount}/{session.setCount} sets
            </p>
          </div>
          {#if (session.exercises ?? []).length > 0}
            <table class="mt-3 w-full text-sm">
              <tbody>
                {#each session.exercises ?? [] as ex (ex.exerciseName)}
                  <tr>
                    <td class="py-0.5 pr-4 font-medium text-card-foreground">
                      {ex.exerciseName}
                    </td>
                    <td class="py-0.5 pr-4 tabular-nums text-muted-foreground">
                      {ex.sets}×{ex.reps}
                    </td>
                    <td class="py-0.5 text-right tabular-nums text-muted-foreground">
                      {ex.weightLb} lb
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </a>
        <!-- Outside the card's own link, because an anchor cannot nest inside
             another one. -->
        {#if hasFooter(session)}
          <div
            class="mt-2 flex items-baseline justify-between gap-4 border-t border-border/60 pt-2"
          >
            <span class="text-xs tabular-nums text-muted-foreground">
              {#if session.volumeLb > 0}{formatVolume(session.volumeLb)} lb lifted{/if}
            </span>
            <!-- Only on your own rows: on somebody else's the whole card is
                 already the recap link, and a second one to the same place
                 would be a control that does nothing new. -->
            {#if mine && session.isOver}
              <a
                use:link
                href="/sessions/{session.id}/recap"
                class="shrink-0 text-xs font-semibold text-primary hover:underline"
              >
                Recap
              </a>
            {/if}
          </div>
        {/if}
      </Card>
    </li>
  {/each}
</ul>
