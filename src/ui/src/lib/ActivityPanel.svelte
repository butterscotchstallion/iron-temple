<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import ErrorBanner from "./ErrorBanner.svelte";
  import {
    backfillActivity,
    deleteActivity,
    getActivityStatus,
    startActivity,
    stopActivity,
    type ActivityStatus,
  } from "./api";

  // Generated training activity, on the owner's account screen.
  //
  // Only reachable by the account that claimed the install: the route condition on
  // /admin keeps the link and the hash from working for anybody else, and every
  // endpoint below sits inside the API's /admin subtree, which is the check that
  // actually counts.
  //
  // Nothing about the lifters this creates is marked anywhere — not in the schema,
  // not in any response — so this panel is the only place in the app that knows
  // they are not people who signed up. Which is why teardown works by re-deriving
  // the roster that named them rather than by looking up a flag, and why it is
  // behind a confirmation: it deletes accounts, and the sessions, reactions and
  // comments that cascade from them.
  let status = $state<ActivityStatus | null>(null);
  let busy = $state(false);
  let error = $state<string | null>(null);
  let note = $state<string | null>(null);
  let confirmingTeardown = $state(false);

  // Defaults chosen to be immediately useful rather than minimal: four lifters over
  // twelve weeks is enough for a monthly leaderboard to rank, a heatmap to have
  // shape and a feed to need paging.
  let lifters = $state(4);
  let weeks = $state(12);
  let tickSeconds = $state(20);

  const maxLifters = $derived(status?.maxLifters ?? 8);
  const maxWeeks = $derived(status?.maxWeeks ?? 26);
  const running = $derived(status?.running === true);

  // Polled only while a loop is running, and that is the whole reason to poll: the
  // action count is how the screen shows the thing is alive rather than merely
  // flagged as on. Idle, there is nothing to watch, so nothing is asked for.
  let poll: ReturnType<typeof setInterval> | null = null;

  function schedulePoll() {
    if (poll !== null) {
      clearInterval(poll);
      poll = null;
    }
    if (running) {
      poll = setInterval(() => void load(), 5000);
    }
  }

  async function load() {
    const result = await getActivityStatus();
    if (result.status === 200) {
      status = result.data;
      schedulePoll();
    }
  }

  // Every action funnels through here so that `busy` and the two message slots
  // cannot get out of step — five call sites each clearing their own would be five
  // places for a stale error to survive a successful retry.
  async function act(what: () => Promise<boolean>, success = "") {
    if (busy) return;
    busy = true;
    error = null;
    note = null;
    const ok = await what();
    busy = false;
    if (!ok) {
      error = "That didn't work. Check the server log.";
      return;
    }
    // Only when the action did not write its own. Backfill and teardown report
    // counts they alone know, and an unconditional assignment here would throw
    // those away for a generic "done" — which is exactly what it used to do.
    if (note === null && success !== "") note = success;
    await load();
  }

  const backfill = () =>
    act(async () => {
      const result = await backfillActivity({ lifters, weeks });
      if (result.status !== 200) return false;
      const s = result.data;
      note =
        `${s.accounts} new ${s.accounts === 1 ? "lifter" : "lifters"}, ` +
        `${s.sessions} sessions, ${s.reactions} reactions, ${s.comments} comments.`;
      return true;
    });

  const start = () =>
    act(async () => (await startActivity({ lifters, tickSeconds })).status === 204, "Running.");

  const stop = () => act(async () => (await stopActivity()).status === 204, "Stopped.");

  const teardown = () =>
    act(async () => {
      const result = await deleteActivity();
      if (result.status !== 200) return false;
      note = `Removed ${result.data.removed} ${result.data.removed === 1 ? "account" : "accounts"}.`;
      return true;
    });

  onMount(load);
  onDestroy(() => {
    if (poll !== null) clearInterval(poll);
  });

  const inputClass =
    "w-24 rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary";
  const labelClass =
    "text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground";
</script>

<Card class="p-6" data-testid="activity-panel">
  <h3 class="text-lg font-bold text-card-foreground">Generated activity</h3>
  <p class="mt-1 text-sm text-muted-foreground">
    Fills the install with lifters and training history, so the screens built for a
    shared gym have something on them. Weights come from the real progression engine,
    so the history stalls and deloads like anyone's would.
  </p>

  {#if error}
    <div class="mt-4">
      <ErrorBanner message={error} onDismiss={() => (error = null)} />
    </div>
  {/if}

  {#if note}
    <p class="mt-4 text-sm text-muted-foreground" role="status">{note}</p>
  {/if}

  <div class="mt-4 flex flex-wrap items-end gap-3">
    <label class="flex flex-col gap-1.5">
      <span class={labelClass}>Lifters</span>
      <input
        bind:value={lifters}
        type="number"
        min="1"
        max={maxLifters}
        class={inputClass}
        aria-label="Lifters"
      />
    </label>
    <label class="flex flex-col gap-1.5">
      <span class={labelClass}>Weeks</span>
      <input
        bind:value={weeks}
        type="number"
        min="1"
        max={maxWeeks}
        class={inputClass}
        aria-label="Weeks"
      />
    </label>
    <Button onclick={backfill} disabled={busy}>
      {busy ? "Working…" : "Generate history"}
    </Button>
  </div>

  <div class="mt-6 border-t border-border/60 pt-4">
    <h4 class={labelClass}>Live</h4>
    <p class="mt-1 text-sm text-muted-foreground">
      One lifter acts per tick — a session, a reaction or a comment — so things arrive
      while a screen is open.
    </p>

    <div class="mt-3 flex flex-wrap items-end gap-3">
      <label class="flex flex-col gap-1.5">
        <span class={labelClass}>Tick (s)</span>
        <input
          bind:value={tickSeconds}
          type="number"
          min="5"
          max="3600"
          class={inputClass}
          aria-label="Tick seconds"
        />
      </label>
      {#if running}
        <Button variant="outline" onclick={stop} disabled={busy}>Stop</Button>
      {:else}
        <Button variant="outline" onclick={start} disabled={busy}>Start</Button>
      {/if}
    </div>

    {#if running && status}
      <p class="mt-3 text-sm text-muted-foreground" role="status">
        Running every {status.tickSeconds}s across {status.lifters}
        {status.lifters === 1 ? "lifter" : "lifters"} · {status.actions}
        {status.actions === 1 ? "action" : "actions"}{status.lastAction
          ? ` · ${status.lastAction}`
          : ""}
      </p>
    {/if}
  </div>

  <div class="mt-6 border-t border-border/60 pt-4">
    <h4 class={labelClass}>Clean up</h4>
    <p class="mt-1 text-sm text-muted-foreground">
      Deletes the generated lifters and everything of theirs — sessions, reactions,
      comments. Your own account and anyone you created by hand are left alone.
    </p>
    <Button
      variant="outline"
      class="mt-3"
      onclick={() => (confirmingTeardown = true)}
      disabled={busy}
    >
      Remove generated lifters
    </Button>
  </div>
</Card>

<!-- Behind a confirmation because it deletes accounts, and the cascade takes their
     whole history with them. Same alert-dialog the set-removal prompt uses. -->
<AlertDialog.Root
  open={confirmingTeardown}
  onOpenChange={(open) => {
    if (!open) confirmingTeardown = false;
  }}
>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>Remove the generated lifters?</AlertDialog.Title>
      <AlertDialog.Description>
        Their accounts go, and so does everything of theirs — every session, set,
        reaction and comment. Your own account and anyone you added by hand are not
        touched. This can't be undone, but you can generate more afterwards.
      </AlertDialog.Description>
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>Keep them</AlertDialog.Cancel>
      <AlertDialog.Action
        onclick={() => {
          confirmingTeardown = false;
          void teardown();
        }}
      >
        Remove
      </AlertDialog.Action>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
