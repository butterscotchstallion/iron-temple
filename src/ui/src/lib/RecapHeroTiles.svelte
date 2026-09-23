<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { formatSessionLength } from "./racked";
  import { formatVolume } from "./volume";
  import { formatOutOf } from "./recap";

  // The four numbers a lifter wants before they have read anything else. Same
  // tile treatment as the Racked page, deliberately — this is the same kind of
  // fact, and two recaps in one app should not have two visual languages.

  let {
    durationSeconds,
    volumeLb,
    setsLogged,
    setsPrescribed,
    repsLogged,
    repsTargeted,
    setsBonus,
  }: {
    /** Null when the session was never finished, or ran past the 12-hour cap. */
    durationSeconds: number | null;
    volumeLb: number;
    setsLogged: number;
    setsPrescribed: number;
    repsLogged: number;
    repsTargeted: number;
    /**
     * Sets added after the rest of the session was already done. Counted inside
     * setsLogged, so this qualifies the Sets tile rather than adding to it.
     */
    setsBonus: number;
  } = $props();
</script>

<section class="grid grid-cols-2 gap-3 sm:grid-cols-4">
  <Card class="p-4 text-center" data-testid="stat-duration">
    <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Duration</h3>
    <p class="text-2xl font-black tabular-nums text-foreground">
      {durationSeconds ? formatSessionLength(durationSeconds) : "—"}
    </p>
  </Card>
  <Card class="p-4 text-center" data-testid="stat-volume">
    <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Volume</h3>
    <p class="text-2xl font-black tabular-nums text-foreground">
      {formatVolume(volumeLb)}<span class="text-base font-bold"> lb</span>
    </p>
  </Card>
  <Card class="p-4 text-center" data-testid="stat-sets">
    <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Sets</h3>
    <p class="text-2xl font-black tabular-nums text-foreground">
      {formatOutOf(setsLogged, setsPrescribed)}
    </p>
    <!-- Work the lifter added once the session was otherwise done. It qualifies
         the number above rather than adding to it — the bonus sets are already
         in that count — so it sits under the tile instead of beside it, and is
         absent rather than "0 bonus" on an ordinary session. -->
    {#if setsBonus > 0}
      <p class="text-xs tabular-nums text-primary" data-testid="stat-sets-bonus">
        {setsBonus} bonus
      </p>
    {/if}
  </Card>
  <Card class="p-4 text-center" data-testid="stat-reps">
    <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Reps</h3>
    <p class="text-2xl font-black tabular-nums text-foreground">
      {formatOutOf(repsLogged, repsTargeted)}
    </p>
  </Card>
</section>
