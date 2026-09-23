<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import {
    createProgram,
    listPrograms,
    type ProgramSummary,
  } from "../lib/api";
  import { CACHE_KEYS, invalidate } from "../lib/cache.svelte";
  import { cloneSources } from "../lib/programs";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import ChevronLeft from "@lucide/svelte/icons/chevron-left";
  import Plus from "@lucide/svelte/icons/plus";

  // Building a program.
  //
  // A screen of its own rather than a dialog, because the choice it opens on —
  // start from nothing, or start from a program that already works — is the
  // whole decision, and it deserves more room than a modal gives it. Most
  // lifters want StrongLifts with one lift swapped rather than a blank page, so
  // the copy leads with that.

  const BLANK = 0;

  let name = $state("");
  let description = $state("");
  let isShared = $state(false);
  let cloneFrom = $state(BLANK);

  let sources = $state<ProgramSummary[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state<string | null>(null);

  const fieldClass =
    "rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none transition focus:border-primary";

  async function load() {
    const list = await listPrograms();
    // A failure here is not fatal and must not block the screen: the picker is
    // an optional starting point, and a lifter who came here to build something
    // from scratch should not be stopped by a list they were not going to use.
    sources = list.status === 200 ? cloneSources(list.data) : [];
    loading = false;
  }

  onMount(load);

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    const trimmed = name.trim();
    if (trimmed === "" || saving) return;

    saving = true;
    error = null;
    const created = await createProgram({
      name: trimmed,
      description: description.trim(),
      isShared,
      // null rather than 0 — the API reads absent as "start from nothing", and
      // 0 is not a program id.
      cloneFromProgramId: cloneFrom === BLANK ? null : cloneFrom,
    });
    saving = false;

    if (created.status !== 201) {
      // The server's message names the reason — a taken name, or a source that
      // prescribes ramps — which is more use to the lifter than "couldn't
      // create".
      error = created.data?.message ?? "Couldn't create that program.";
      return;
    }

    // The picker is now a list missing the program that was just built, so drop
    // it. Home reads the current program from the same place.
    invalidate(CACHE_KEYS.allExercises);
    push(`/programs/${created.data.id}`);
  }
</script>

<div class="flex flex-col gap-4">
  <a
    use:link
    href="/programs"
    class="flex items-center gap-1 self-start text-sm text-muted-foreground transition hover:text-foreground"
  >
    <ChevronLeft class="size-4" aria-hidden="true" />
    Programs
  </a>

  <div>
    <h2 class="text-2xl font-black text-foreground">Build a program</h2>
    <p class="mt-1 text-sm text-muted-foreground">
      Start from one that already works and change what you need, or from
      nothing at all. Either way your lifts keep their history — a squat is a
      squat, so it picks up where you left it rather than starting over.
    </p>
  </div>

  {#if error}
    <ErrorBanner message={error} onDismiss={() => (error = null)} />
  {/if}

  <Card class="p-5">
    <form class="flex flex-col gap-4" onsubmit={submit}>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">Name</span>
        <input
          bind:value={name}
          maxlength="80"
          required
          placeholder="Push Pull Legs"
          class={fieldClass}
        />
      </label>

      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">Description <span class="text-muted-foreground/60">(optional)</span></span>
        <input
          bind:value={description}
          maxlength="500"
          placeholder="What this is for"
          class={fieldClass}
        />
      </label>

      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">Start from</span>
        <!-- No skeleton here, and that is the shape of the screen rather than an
             omission: the form is the same height before and after the list
             lands, because all that arrives is <option>s inside a control that
             is already drawn. What WAS missing is any sign the control is
             temporarily inert — `disabled` alone reads as "you can't do this"
             rather than "not yet", and says nothing at all to a screen reader.
             So the first option says what it is waiting for, and aria-busy says
             the same thing to anything listening. -->
        <select
          bind:value={cloneFrom}
          class={fieldClass}
          disabled={loading}
          aria-busy={loading}
        >
          <option value={BLANK}>
            {loading ? "Loading programs…" : "Nothing — an empty program"}
          </option>
          {#each sources as source (source.id)}
            <option value={source.id}>{source.name}</option>
          {/each}
        </select>
        <span class="text-xs text-muted-foreground/80">
          A copy takes the days and the lifts on them. It doesn't take the
          weekdays — when you train is yours to decide — and it leaves the
          original untouched.
        </span>
      </label>

      <label class="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          bind:checked={isShared}
          class="mt-0.5 size-4 rounded border-input"
        />
        <span class="flex flex-col gap-0.5">
          <span class="text-card-foreground">Share it with the other lifters here</span>
          <span class="text-xs text-muted-foreground/80">
            They'll see it in their picker and can train it. Only you can change
            it. You can turn this off later, though anyone already running it
            keeps it.
          </span>
        </span>
      </label>

      <div class="flex gap-2">
        <Button type="submit" disabled={saving || name.trim() === ""}>
          <Plus />
          {saving ? "Creating…" : "Create program"}
        </Button>
        <a use:link href="/programs">
          <Button type="button" variant="ghost">Cancel</Button>
        </a>
      </div>
    </form>
  </Card>
</div>
