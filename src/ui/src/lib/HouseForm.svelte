<script lang="ts">
  import { untrack } from "svelte";
  import { Button } from "$lib/components/ui/button";
  import { Textarea } from "$lib/components/ui/textarea";
  import ErrorBanner from "./ErrorBanner.svelte";
  import HouseIconGlyph from "./HouseIcon.svelte";
  import { HouseIcon, type CreateHouseRequest } from "./api";

  // The form behind both founding a House and editing one.
  //
  // One component for both, so a ceiling enforced on the way in cannot be
  // forgotten on the way through — the same reason validateHouseFields in
  // houses.go is one function rather than two. The maxlengths here match its
  // limits, which match the CHECK constraints in 0033. Three copies of a number
  // is two too many, and the one that matters is the CHECK; these two exist so a
  // lifter is stopped by the field rather than by a 400.
  let {
    initial,
    saving = false,
    error = null,
    submitLabel,
    onsubmit,
    oncancel,
  }: {
    initial?: CreateHouseRequest;
    saving?: boolean;
    error?: string | null;
    submitLabel: string;
    onsubmit: (body: CreateHouseRequest) => void;
    oncancel?: () => void;
  } = $props();

  const FIELD =
    "rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary";
  const LABEL = "text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground";

  // The same palette the initials chip offers, because a House's icon and a
  // lifter's chip sit next to each other on this install and a colour that looks
  // deliberate in one should look deliberate in the other.
  const COLORS = ["", "#b026ff", "#ff2fb9", "#05d9e8", "#ff6ac1", "#7b2ff7"];

  // Straight off the generated enum rather than a list written out here. The spec
  // is the set — see the HouseIcon schema — so a name added there shows up in this
  // picker without anybody remembering to add it.
  const ICON_NAMES = Object.values(HouseIcon);

  // Seeded once, untracked. Capturing the initial value IS the intent — the same
  // reason DetailsSection untracks the profile it edits: a reactive seed would
  // take the server's answer from a save and overwrite whatever the owner had
  // typed since. Closing the form is how an unwanted edit is discarded.
  let name = $state(untrack(() => initial?.name) ?? "");
  let sigil = $state(untrack(() => initial?.sigil) ?? "");
  let tagline = $state(untrack(() => initial?.tagline) ?? "");
  let description = $state(untrack(() => initial?.description) ?? "");
  let icon = $state<HouseIcon>(untrack(() => initial?.icon) ?? "");
  let iconColor = $state(untrack(() => initial?.iconColor) ?? "");

  // A stand-in House for the icon preview, which wants an id to derive a colour
  // from when none is chosen. 0 is not a real House and does not have to be: the
  // derivation only uses it to index a palette.
  const preview = $derived({ id: 0, icon, iconColor });

  function submit(event: SubmitEvent) {
    event.preventDefault();
    onsubmit({
      name: name.trim(),
      sigil: sigil.trim(),
      tagline: tagline.trim(),
      description,
      icon,
      iconColor,
    });
  }
</script>

<form class="mt-4 flex flex-col gap-4" onsubmit={submit}>
  {#if error}
    <ErrorBanner message={error} />
  {/if}

  <label class="flex flex-col gap-1.5">
    <span class={LABEL}>Name</span>
    <input bind:value={name} maxlength="60" required class={FIELD} />
  </label>

  <label class="flex flex-col gap-1.5">
    <span class={LABEL}>Sigil</span>
    <input
      bind:value={sigil}
      maxlength="5"
      minlength="2"
      pattern="[A-Za-z0-9]&#123;2,5&#125;"
      required
      class="{FIELD} w-24 uppercase"
    />
    <span class="text-xs text-muted-foreground">
      Two to five letters or digits. Worn beside every member's name, so short is
      better — a long one pushes the name out of the row.
    </span>
  </label>

  <label class="flex flex-col gap-1.5">
    <span class={LABEL}>Tagline</span>
    <input bind:value={tagline} maxlength="80" class={FIELD} />
    <span class="text-xs text-muted-foreground">
      One line, shown when somebody hovers the sigil.
    </span>
  </label>

  <label class="flex flex-col gap-1.5">
    <span class={LABEL}>Description</span>
    <Textarea bind:value={description} maxlength={2000} rows={4} class={FIELD} />
    <span class="text-xs text-muted-foreground">
      The longer version, shown on the House's page.
    </span>
  </label>

  <fieldset class="flex flex-col gap-2">
    <legend class={LABEL}>Icon</legend>
    <div class="flex flex-wrap gap-2">
      {#each ICON_NAMES as choice (choice)}
        <label
          class="cursor-pointer rounded-md p-1.5 ring-2 transition {icon === choice
            ? 'ring-primary'
            : 'ring-transparent hover:ring-border'}"
        >
          <input
            type="radio"
            name="icon"
            value={choice}
            bind:group={icon}
            class="sr-only"
          />
          {#if choice === ""}
            <span class="block size-5 text-center text-[0.6rem] leading-5 text-muted-foreground">
              None
            </span>
          {:else}
            <HouseIconGlyph house={{ id: 0, icon: choice, iconColor }} />
          {/if}
          <span class="sr-only">{choice || "No icon"}</span>
        </label>
      {/each}
    </div>
  </fieldset>

  <fieldset class="flex flex-col gap-2">
    <legend class={LABEL}>Icon colour</legend>
    <div class="flex flex-wrap items-center gap-2">
      {#each COLORS as colour (colour)}
        <label
          class="cursor-pointer rounded-full p-0.5 ring-2 transition {iconColor === colour
            ? 'ring-primary'
            : 'ring-transparent hover:ring-border'}"
        >
          <input
            type="radio"
            name="iconColor"
            value={colour}
            bind:group={iconColor}
            class="sr-only"
          />
          <span
            class="block size-7 rounded-full border border-border/60 text-center text-[0.6rem] leading-7 text-ink"
            style={colour ? `background-color:${colour}` : ""}
          >
            {colour ? "" : "Auto"}
          </span>
        </label>
      {/each}
      {#if icon}
        <span class="ml-1 inline-flex items-center gap-1 text-xs text-muted-foreground">
          <HouseIconGlyph house={preview} size="size-6" />
        </span>
      {/if}
    </div>
  </fieldset>

  <div class="flex items-center gap-3">
    <Button type="submit" disabled={saving}>
      {saving ? "Saving…" : submitLabel}
    </Button>
    {#if oncancel}
      <Button type="button" variant="ghost" onclick={oncancel} disabled={saving}>
        Cancel
      </Button>
    {/if}
  </div>
</form>
