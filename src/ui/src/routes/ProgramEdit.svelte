<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    getProgram,
    updateProgram,
    addProgramDay,
    updateProgramDay,
    removeProgramDay,
    reorderProgramDays,
    addPrescription,
    removePrescription,
    reorderPrescriptions,
    archiveProgram,
    unarchiveProgram,
    type Program,
    type ProgramDay,
  } from "../lib/api";
  import { invalidateTraining } from "../lib/cache.svelte";
  import {
    idsInOrder,
    move,
    prescribedExerciseIds,
    prescriptionSummary,
    validName,
  } from "../lib/programEdit";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import Loading from "../lib/skeleton/Loading.svelte";
  import Skeleton from "../lib/skeleton/Skeleton.svelte";
  import PrescriptionPicker from "../lib/PrescriptionPicker.svelte";
  import OrderControls from "../lib/OrderControls.svelte";
  import ChevronLeft from "@lucide/svelte/icons/chevron-left";
  import Plus from "@lucide/svelte/icons/plus";
  import Trash2 from "@lucide/svelte/icons/trash-2";

  // Editing the structure of a program you own.
  //
  // A route of its own rather than a mode inside ProgramDetail, which is already
  // a thousand lines of "what am I lifting today" — the deload prompt, the
  // layoff question, weekday pickers, starting weights and assistance. This is a
  // different question ("what IS this program"), asked far less often, and
  // mixing the two would leave one screen answering both badly.
  //
  // Every write here answers with the whole program, so there is no local
  // reconciliation to get wrong: the server decides positions, and the screen
  // redraws from what it says.

  let { params }: { params?: { id?: string } } = $props();
  let programId = $derived(Number(params?.id));

  // The loading placeholder: name, description and the share tickbox, then a
  // card per workout day carrying its lifts. Counts matched to a program built
  // here — the editor is reached from a program you own, and the ones lifters
  // build run to three or four days.
  const SKELETON_FIELDS = [0, 1, 2];
  const SKELETON_DAYS = [0, 1, 2];
  const SKELETON_LIFTS = [0, 1, 2, 3];

  let program = $state<Program | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let error = $state<string | null>(null);

  // The metadata form, held separately from `program` so an unsaved edit is not
  // wiped by a reload triggered from elsewhere on the screen.
  let name = $state("");
  let description = $state("");
  let isShared = $state(false);
  let savingMeta = $state(false);
  let metaSaved = $state(false);
  // Any write on this screen is in flight. See write() for why it is one flag.
  let writing = $state(false);

  let newDayName = $state("");
  let addingDayTo = $state<number | null>(null);
  let confirmRemoveDay = $state<ProgramDay | null>(null);
  let confirmArchive = $state(false);

  const fieldClass =
    "rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none transition focus:border-primary";

  async function load() {
    // Only blank the screen when there is nothing on it yet. This also runs as a
    // REFRESH — renaming a day and removing one both answer 204, so the program
    // has to be re-read — and an unconditional `true` meant those replaced the
    // whole editor with its placeholder and put it back a moment later. That was
    // survivable while the placeholder was one small card; against a
    // full-height one it would flash the page out from under the rename that
    // caused it.
    loading = program === null;
    failed = false;
    const result = await getProgram(programId);
    loading = false;
    if (result.status !== 200) {
      failed = true;
      return;
    }
    // The API is the boundary and will refuse every write regardless, but a
    // screen full of controls that all 404 is a worse way to say "this isn't
    // yours" than not opening it.
    if (!result.data.isMine) {
      push(`/programs/${programId}`);
      return;
    }
    apply(result.data);
  }

  function apply(next: Program) {
    program = next;
    name = next.name;
    description = next.description;
    isShared = next.isShared;
  }

  onMount(load);

  /**
   * Runs a write and redraws from its response.
   *
   * Every editing endpoint answers with the whole program for exactly this
   * reason — after adding a day the screen needs its id, its position among the
   * others and the empty prescription it starts with, and rebuilding that
   * locally is how two views of one program start to disagree.
   */
  async function write<T extends { status: number; data?: unknown }>(
    call: () => Promise<T>,
    fallback: string,
    expected: number[] = [200, 201],
  ): Promise<boolean> {
    error = null;
    // One flag for every write on this screen, set here rather than at the six
    // call sites. Each of them redraws the whole program from the response, so
    // any second write started before the first lands is aimed at positions and
    // ids that are about to be replaced — and every one of these controls was
    // live throughout, with nothing on screen to say a request was out.
    writing = true;
    const result = await call();
    writing = false;
    if (!expected.includes(result.status)) {
      // The server's message names the actual reason — a taken name, a lift
      // already on the day — which is more use than a generic failure.
      error =
        (result.data as { message?: string } | undefined)?.message ?? fallback;
      return false;
    }
    if (result.data && typeof result.data === "object" && "days" in result.data) {
      apply(result.data as Program);
    }
    // The program's shape decides what Home's heatmap draws and what the picker
    // shows, so the cached copies are now stale.
    invalidateTraining();
    return true;
  }

  async function saveMeta(event: SubmitEvent) {
    event.preventDefault();
    if (!validName(name) || savingMeta) return;
    savingMeta = true;
    metaSaved = false;
    const ok = await write(
      () => updateProgram(programId, { name: name.trim(), description: description.trim(), isShared }),
      "Couldn't save those details.",
    );
    savingMeta = false;
    metaSaved = ok;
  }

  async function addDay(event: SubmitEvent) {
    event.preventDefault();
    if (!validName(newDayName)) return;
    if (await write(() => addProgramDay(programId, { name: newDayName.trim() }), "Couldn't add that day.")) {
      newDayName = "";
    }
  }

  async function renameDay(day: ProgramDay, value: string) {
    if (!validName(value) || value.trim() === day.name) return;
    await write(
      () => updateProgramDay(programId, day.id, { name: value.trim() }),
      "Couldn't rename that day.",
      [204],
    );
    // 204 carries no program, so re-read to pick the new name up.
    await load();
  }

  async function confirmRemoveDayNow() {
    const day = confirmRemoveDay;
    confirmRemoveDay = null;
    if (!day) return;
    await write(
      () => removeProgramDay(programId, day.id),
      "Couldn't remove that day.",
      [204],
    );
    await load();
  }

  async function moveDay(day: ProgramDay, direction: "up" | "down") {
    if (!program) return;
    const next = move(program.days, day.id, direction);
    // move() returns the same array when the move is impossible, so a disabled
    // button that somehow fires costs nothing rather than sending a 400.
    if (next === program.days) return;
    await write(
      () => reorderProgramDays(programId, { ids: idsInOrder(next) }),
      "Couldn't reorder the days.",
    );
  }

  async function moveLift(day: ProgramDay, liftId: number, direction: "up" | "down") {
    const next = move(day.exercises, liftId, direction);
    if (next === day.exercises) return;
    await write(
      () => reorderPrescriptions(programId, day.id, { ids: idsInOrder(next) }),
      "Couldn't reorder the lifts.",
    );
  }

  async function addLift(
    day: ProgramDay,
    choice: {
      exerciseId: number;
      sets: number;
      reps: number;
      startingWeightLb: number;
    },
  ): Promise<boolean> {
    const ok = await write(
      () => addPrescription(programId, day.id, choice),
      "Couldn't add that lift.",
    );
    if (ok) addingDayTo = null;
    return ok;
  }

  async function removeLift(day: ProgramDay, liftId: number) {
    // No confirmation, matching how assistance is removed: this loses a plan and
    // never a performance — every set ever logged for the movement stays in the
    // lifter's history — and re-adding it is one tap away.
    await write(
      () => removePrescription(programId, day.id, liftId),
      "Couldn't remove that lift.",
      [204],
    );
    await load();
  }

  async function toggleArchive() {
    confirmArchive = false;
    if (!program) return;
    const archived = program.archivedAt !== null;
    await write(
      () => (archived ? unarchiveProgram(programId) : archiveProgram(programId)),
      archived ? "Couldn't bring that program back." : "Couldn't archive that program.",
      [204],
    );
    await load();
  }
</script>

<div class="flex flex-col gap-4">
  <a
    use:link
    href="/programs/{programId}"
    class="flex items-center gap-1 self-start text-sm text-muted-foreground transition hover:text-neon-lift"
  >
    <ChevronLeft class="size-4" aria-hidden="true" />
    Back to the program
  </a>

  {#if loading}
    <!-- The heading, the details form and a card per day. A single h-40 box used
         to stand in for the lot, so opening the editor moved everything below it
         by most of a screen once the program arrived. -->
    <Loading label="Loading this program" class="flex flex-col gap-4">
      <Skeleton text="2xl" class="w-56" />
      <Card class="p-5">
        <div class="flex flex-col gap-3">
          {#each SKELETON_FIELDS as field (field)}
            <div class="flex flex-col gap-1">
              <Skeleton text="sm" class="w-20" />
              <Skeleton class="h-[38px] w-full" />
            </div>
          {/each}
          <Skeleton class="h-8 w-16 rounded-md" />
        </div>
      </Card>
      <Skeleton text="lg" class="w-16" />
      {#each SKELETON_DAYS as day (day)}
        <Card class="p-5">
          <Skeleton class="h-[38px] w-full" />
          <ul class="mt-3 flex flex-col gap-1.5">
            {#each SKELETON_LIFTS as lift (lift)}
              <li class="flex flex-col">
                <Skeleton class="h-[18px] w-40" />
                <Skeleton class="h-[15px] w-28" />
              </li>
            {/each}
          </ul>
        </Card>
      {/each}
    </Loading>
  {:else if failed || !program}
    <ErrorCard message="Couldn't load this program." onRetry={load} />
  {:else}
    <h2 class="text-2xl font-black text-foreground">Edit {program.name}</h2>

    {#if error}
      <ErrorBanner message={error} onDismiss={() => (error = null)} />
    {/if}

    <Card class="p-5">
      <form class="flex flex-col gap-3" onsubmit={saveMeta}>
        <label class="flex flex-col gap-1 text-sm">
          <span class="text-muted-foreground">Name</span>
          <input bind:value={name} maxlength="80" required class={fieldClass} />
        </label>
        <label class="flex flex-col gap-1 text-sm">
          <span class="text-muted-foreground">Description</span>
          <input bind:value={description} maxlength="500" class={fieldClass} />
        </label>
        <label class="flex items-start gap-2 text-sm">
          <input type="checkbox" bind:checked={isShared} class="mt-0.5 size-4" />
          <span class="flex flex-col gap-0.5">
            <span class="text-card-foreground">Share it with the other lifters here</span>
            <span class="text-xs text-muted-foreground/80">
              Turning this off takes it off their picker. Anyone who's already
              trained it keeps it — their sessions are tied to it either way.
            </span>
          </span>
        </label>
        <div class="flex items-center gap-3">
          <Button type="submit" size="sm" disabled={savingMeta || !validName(name)}>
            {savingMeta ? "Saving…" : "Save"}
          </Button>
          {#if metaSaved}
            <span role="status" class="text-sm text-muted-foreground">Saved.</span>
          {/if}
        </div>
      </form>
    </Card>

    <h3 class="text-lg font-bold text-foreground">Days</h3>

    {#each program.days as day, dayIndex (day.id)}
      <Card class="p-5">
        <div class="flex items-start gap-2">
          <input
            value={day.name}
            maxlength="80"
            aria-label="Day name"
            class="{fieldClass} flex-1 font-bold"
            onchange={(e) => renameDay(day, e.currentTarget.value)}
          />
          <OrderControls
            label={day.name}
            isFirst={dayIndex === 0}
            isLast={dayIndex === program.days.length - 1}
            busy={writing}
            onUp={() => moveDay(day, "up")}
            onDown={() => moveDay(day, "down")}
          />
          <button
            type="button"
            aria-label="Remove {day.name}"
            class="rounded-md p-1.5 text-muted-foreground transition hover:text-destructive disabled:opacity-40"
            disabled={writing}
            onclick={() => (confirmRemoveDay = day)}
          >
            <Trash2 class="size-4" aria-hidden="true" />
          </button>
        </div>

        {#if day.exercises.length > 0}
          <ul class="mt-3 flex flex-col gap-1.5">
            {#each day.exercises as lift, liftIndex (lift.id)}
              <li class="flex items-center gap-2 text-sm">
                <span class="flex-1 leading-tight">
                  <span class="block font-medium text-card-foreground">
                    {lift.exerciseName}
                  </span>
                  <span class="block text-xs tabular-nums text-muted-foreground">
                    {prescriptionSummary(lift)}
                  </span>
                </span>
                <OrderControls
                  label={lift.exerciseName}
                  isFirst={liftIndex === 0}
                  isLast={liftIndex === day.exercises.length - 1}
                  busy={writing}
                  onUp={() => moveLift(day, lift.id, "up")}
                  onDown={() => moveLift(day, lift.id, "down")}
                />
                <button
                  type="button"
                  aria-label="Remove {lift.exerciseName}"
                  class="rounded-md p-1.5 text-muted-foreground transition hover:text-destructive disabled:opacity-40"
                  disabled={writing}
                  onclick={() => removeLift(day, lift.id)}
                >
                  <Trash2 class="size-4" aria-hidden="true" />
                </button>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="mt-3 text-sm text-muted-foreground">
            Nothing on this day yet.
          </p>
        {/if}

        {#if addingDayTo === day.id}
          <PrescriptionPicker
            exclude={prescribedExerciseIds(day)}
            onAdd={(choice) => addLift(day, choice)}
            onCancel={() => (addingDayTo = null)}
          />
        {:else}
          <Button
            size="sm"
            variant="outline"
            class="mt-3"
            onclick={() => (addingDayTo = day.id)}
          >
            <Plus />
            Add a lift
          </Button>
        {/if}
      </Card>
    {/each}

    <Card class="p-5">
      <form class="flex flex-wrap items-end gap-2" onsubmit={addDay}>
        <label class="flex flex-1 flex-col gap-1 text-sm">
          <span class="text-muted-foreground">New day</span>
          <input
            bind:value={newDayName}
            maxlength="80"
            placeholder="Workout A"
            class={fieldClass}
          />
        </label>
        <Button type="submit" size="sm" disabled={!validName(newDayName)}>
          <Plus />
          Add day
        </Button>
      </form>
    </Card>

    <div class="flex flex-col gap-2 border-t border-border/60 pt-4">
      <Button
        variant="outline"
        size="sm"
        class="self-start"
        onclick={() =>
          program?.archivedAt !== null ? toggleArchive() : (confirmArchive = true)}
      >
        {program.archivedAt !== null ? "Bring this program back" : "Archive this program"}
      </Button>
      <p class="text-xs text-muted-foreground">
        Archiving takes it off the picker. Everything you've logged against it
        stays in your history — which is why it can't simply be deleted.
      </p>
    </div>
  {/if}
</div>

<AlertDialog.Root
  open={confirmRemoveDay !== null}
  onOpenChange={(open) => {
    if (!open) confirmRemoveDay = null;
  }}
>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>Remove {confirmRemoveDay?.name}?</AlertDialog.Title>
      <AlertDialog.Description>
        It comes off the program along with the lifts on it. Any session you've
        already logged against it stays in your history.
      </AlertDialog.Description>
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>Keep it</AlertDialog.Cancel>
      <AlertDialog.Action onclick={confirmRemoveDayNow}>Remove</AlertDialog.Action>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>

<AlertDialog.Root open={confirmArchive} onOpenChange={(o) => (confirmArchive = o)}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>Archive {program?.name}?</AlertDialog.Title>
      <AlertDialog.Description>
        It comes off the picker — yours and, if you've shared it, everyone
        else's. You can still train it, and you can bring it back whenever you
        want.
      </AlertDialog.Description>
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
      <AlertDialog.Action onclick={toggleArchive}>Archive</AlertDialog.Action>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
