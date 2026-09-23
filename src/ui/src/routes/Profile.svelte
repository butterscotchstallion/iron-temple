<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import Download from "@lucide/svelte/icons/download";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import Upload from "@lucide/svelte/icons/upload";
  import Avatar from "../lib/Avatar.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import { auth, setMe } from "../lib/auth.svelte";
  import { downloadAccountExport } from "../lib/accountExport";
  import {
    changePassword,
    deleteAvatar,
    updateMe,
    uploadAvatar,
  } from "../lib/api";

  // Three independent forms, each saving on its own. Grouping them behind one
  // Save button would mean a rejected password change discards an unrelated
  // display-name edit.

  const COLORS = ["", "#b026ff", "#ff2fb9", "#05d9e8", "#ff6ac1", "#7b2ff7"];

  let displayName = $state(auth.me?.displayName ?? "");
  let avatarColor = $state(auth.me?.avatarColor ?? "");
  let profileSaving = $state(false);
  let profileError = $state<string | null>(null);
  let profileSaved = $state(false);

  let currentPassword = $state("");
  let newPassword = $state("");
  let passwordSaving = $state(false);
  let passwordError = $state<string | null>(null);
  let passwordSaved = $state(false);

  let avatarBusy = $state(false);
  let avatarError = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);

  let exporting = $state(false);
  let exportError = $state<string | null>(null);

  // The gym: what the bar weighs and what is in the rack. Both used to be
  // constants in the bundle, and both were wrong — the bar was assumed to be
  // 45 lb, and the plate set was treated as unlimited. Every weight the app
  // draws is loaded onto them, so they belong to the lifter, not to the build.
  let barWeight = $state(auth.me?.barWeightLb ?? 45);
  // A local copy: edits should be discardable by navigating away, and binding
  // straight to auth.me would rewrite the profile as the lifter typed.
  let plates = $state<{ plateLb: number; pairs: number }[]>(
    (auth.me?.plates ?? []).map((p) => ({ ...p })),
  );
  // What the dumbbell rack steps by, per bell — the third thing the gym is made
  // of, and the one that was still a constant in the engine until 0020. A pair
  // moves twice this, so a rack of 5s has nothing between 10 lb and a rack of
  // 2.5s has nothing between 5.
  let dumbbellStep = $state(auth.me?.dumbbellStepLb ?? 5);
  // The three stacks, as whole load. Not halved on the way in the way the rack
  // above is: a lifter holds two bells and moves one pin. Everything that is
  // not a barbell, a dumbbell or one of these keeps taking the bar's grid.
  let machineStep = $state(auth.me?.machineStepLb ?? 5);
  let cableStep = $state(auth.me?.cableStepLb ?? 5);
  let bandStep = $state(auth.me?.bandStepLb ?? 5);
  let gymSaving = $state(false);
  let gymError = $state<string | null>(null);
  let gymSaved = $state(false);

  // Whether this gym was described by its owner or assembled by us.
  //
  // Read once into a local rather than through `auth.me`, so that saving flips
  // the banner off and it stays off — `setMe` refreshes `auth.me`, and a
  // derived value would be correct but would also flicker back on for any
  // future path that clears the profile while this screen is mounted.
  let confirmed = $state(auth.me?.equipmentConfirmedAt != null);

  // The denominations a rack is built from. Owning none of one is normal, so
  // this is the menu rather than a claim about what is there.
  const DENOMINATIONS = [45, 35, 25, 10, 5, 2.5];

  function pairsOf(plateLb: number): number {
    return plates.find((p) => p.plateLb === plateLb)?.pairs ?? 0;
  }

  function setPairs(plateLb: number, pairs: number) {
    const next = Math.max(0, Math.min(20, pairs));
    const existing = plates.find((p) => p.plateLb === plateLb);
    // Zero pairs is an absent row, not a row saying zero — the API rejects
    // pairs < 1, and "I own none" is expressed by not listing it.
    if (next === 0) {
      plates = plates.filter((p) => p.plateLb !== plateLb);
    } else if (existing) {
      existing.pairs = next;
    } else {
      plates = [...plates, { plateLb, pairs: next }].sort(
        (a, b) => b.plateLb - a.plateLb,
      );
    }
  }

  // Hand over the whole account as a file. A failure has to be said out loud:
  // the one thing worse than not having a backup is believing you do.
  async function exportAccount() {
    if (exporting) return;
    exporting = true;
    exportError = null;
    try {
      await downloadAccountExport();
    } catch {
      exportError = "Couldn't build your export. Try again in a moment.";
    } finally {
      exporting = false;
    }
  }

  async function saveGym(event: SubmitEvent) {
    event.preventDefault();
    gymSaving = true;
    gymError = null;
    gymSaved = false;

    const saved = await updateMe({
      barWeightLb: barWeight,
      dumbbellStepLb: dumbbellStep,
      machineStepLb: machineStep,
      cableStepLb: cableStep,
      bandStepLb: bandStep,
      plates,
    });
    if (saved.status !== 200) {
      gymError = "Couldn't save your gym setup.";
    } else {
      setMe(saved.data);
      gymSaved = true;
      // Saving IS the confirmation — a lifter who just edited this has
      // reviewed it. There is no separate "yes, this is right" button, because
      // a button like that is one more thing to not press.
      confirmed = true;
    }
    gymSaving = false;
  }

  async function saveProfile(event: SubmitEvent) {
    event.preventDefault();
    profileSaving = true;
    profileError = null;
    profileSaved = false;

    const saved = await updateMe({ displayName, avatarColor });
    if (saved.status !== 200) {
      profileError = "Couldn't save your profile.";
    } else {
      setMe(saved.data);
      profileSaved = true;
    }
    profileSaving = false;
  }

  async function savePassword(event: SubmitEvent) {
    event.preventDefault();
    passwordSaving = true;
    passwordError = null;
    passwordSaved = false;

    const changed = await changePassword({ currentPassword, newPassword });
    if (changed.status !== 204) {
      // The server distinguishes "wrong current password" (401) from a new
      // password that fails validation (400); both land here as a message the
      // user can act on.
      passwordError =
        "Couldn't change your password. Check your current password and try again.";
    } else {
      passwordSaved = true;
      currentPassword = "";
      newPassword = "";
    }
    passwordSaving = false;
  }

  async function onFileChosen(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;

    avatarBusy = true;
    avatarError = null;
    const uploaded = await uploadAvatar({ avatar: file });
    if (uploaded.status !== 200) {
      avatarError = "Couldn't upload that image. PNG or JPEG, up to 256 KB.";
    } else if (auth.me) {
      // Patch the etag locally so the <img> cache-buster changes and the new
      // picture appears at once, without a round trip to /me.
      setMe({ ...auth.me, hasAvatar: true, avatarEtag: uploaded.data.etag });
    }
    avatarBusy = false;
    // Clear the input so re-picking the same file fires change again.
    input.value = "";
  }

  async function removeAvatar() {
    avatarBusy = true;
    avatarError = null;
    const removed = await deleteAvatar();
    if (removed.status !== 204) {
      avatarError = "Couldn't remove your avatar.";
    } else if (auth.me) {
      setMe({ ...auth.me, hasAvatar: false, avatarEtag: undefined });
    }
    avatarBusy = false;
  }

  const fieldClass =
    "rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary";
  const labelClass =
    "text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground";
</script>

<div class="flex flex-col gap-6">
  <h2 class="text-2xl font-black text-foreground">Profile</h2>

  {#if auth.me}
    <Card class="flex flex-col gap-5 p-6">
      <h3 class="text-lg font-bold text-card-foreground">Avatar</h3>
      {#if avatarError}
        <ErrorBanner message={avatarError} onDismiss={() => (avatarError = null)} />
      {/if}
      <div class="flex items-center gap-5">
        <Avatar user={auth.me} size={72} />
        <div class="flex flex-col gap-2">
          <p class="text-sm text-muted-foreground">
            PNG or JPEG, up to 256 KB and 1024×1024. Without one you get an
            initials chip in the colour below.
          </p>
          <div class="flex flex-wrap gap-2">
            <input
              bind:this={fileInput}
              type="file"
              accept="image/png,image/jpeg"
              class="hidden"
              onchange={onFileChosen}
            />
            <Button
              type="button"
              disabled={avatarBusy}
              onclick={() => fileInput?.click()}
            >
              <Upload />
              {avatarBusy ? "Working…" : "Upload image"}
            </Button>
            {#if auth.me.hasAvatar}
              <Button
                type="button"
                variant="secondary"
                disabled={avatarBusy}
                onclick={removeAvatar}
              >
                <Trash2 />
                Remove
              </Button>
            {/if}
          </div>
        </div>
      </div>
    </Card>

    <Card class="p-6">
      <h3 class="text-lg font-bold text-card-foreground">Details</h3>
      <form class="mt-4 flex flex-col gap-4" onsubmit={saveProfile}>
        {#if profileError}
          <ErrorBanner message={profileError} onDismiss={() => (profileError = null)} />
        {/if}

        <label class="flex flex-col gap-1.5">
          <span class={labelClass}>Username</span>
          <input value={auth.me.username} disabled class="{fieldClass} opacity-60" />
          <span class="text-xs text-muted-foreground">
            Your username can't be changed.
          </span>
        </label>

        <label class="flex flex-col gap-1.5">
          <span class={labelClass}>Display name</span>
          <input bind:value={displayName} maxlength="64" required class={fieldClass} />
        </label>

        <fieldset class="flex flex-col gap-2">
          <legend class={labelClass}>Chip colour</legend>
          <div class="flex flex-wrap gap-2">
            {#each COLORS as colour (colour)}
              <label
                class="cursor-pointer rounded-full p-0.5 ring-2 transition {avatarColor ===
                colour
                  ? 'ring-primary'
                  : 'ring-transparent hover:ring-border'}"
              >
                <input
                  type="radio"
                  name="avatarColor"
                  value={colour}
                  bind:group={avatarColor}
                  class="sr-only"
                />
                <span
                  class="block size-7 rounded-full border border-border/60 text-[0.6rem] leading-7 text-center text-ink"
                  style={colour ? `background-color:${colour}` : ""}
                >
                  {colour ? "" : "Auto"}
                </span>
              </label>
            {/each}
          </div>
        </fieldset>

        <div class="flex items-center gap-3">
          <Button type="submit" disabled={profileSaving}>
            {profileSaving ? "Saving…" : "Save"}
          </Button>
          {#if profileSaved}
            <span class="text-sm text-muted-foreground" role="status">Saved.</span>
          {/if}
        </div>
      </form>
    </Card>

    <Card class="p-6">
      <h3 class="text-lg font-bold text-card-foreground">Your equipment</h3>
      <p class="mt-1 text-sm text-muted-foreground">
        Every weight the app shows is built out of what's here. If any of it is
        wrong, the plate maths, the warm-ups and the jumps between sessions are
        wrong with it.
      </p>

      <!-- Shown until a lifter saves this screen once. The app seeds a standard
           home-gym set at registration, and until then there has been no way to
           tell that guess from a fact — which is how somebody ended up being
           offered a 35 lb plate they have never owned. It is a prompt, not a
           gate: the prescriptions are running off these numbers either way. -->
      {#if !confirmed}
        <p
          class="mt-3 rounded-md border border-primary/40 bg-primary/10 px-3 py-2 text-sm text-card-foreground"
          role="status"
        >
          We've never checked this with you. These are the defaults we set up
          when you joined, not what you told us — have a look and hit Save, even
          if it's all already right.
        </p>
      {/if}

      <form class="mt-4 flex flex-col gap-4" onsubmit={saveGym}>
        {#if gymError}
          <ErrorBanner message={gymError} onDismiss={() => (gymError = null)} />
        {/if}

        <label class="flex flex-col gap-1.5">
          <span class={labelClass}>Bar weight (lb)</span>
          <input
            bind:value={barWeight}
            type="number"
            min="1"
            max="200"
            step="0.5"
            required
            class="{fieldClass} w-32"
          />
          <span class="text-xs text-muted-foreground">
            A standard Olympic bar is 45 lb. Weigh yours if you're not sure.
          </span>
        </label>

        <label class="flex flex-col gap-1.5">
          <span class={labelClass}>Dumbbell step (lb per bell)</span>
          <input
            bind:value={dumbbellStep}
            type="number"
            min="1"
            max="25"
            step="0.25"
            required
            class="{fieldClass} w-32"
          />
          <!-- Spelled out in both units on purpose. The number asked for is the
               one written on the rack, but every weight the app shows is the
               pair, so a lifter typing 5 should see the 10 it becomes rather
               than discover it when a curl jumps further than they expected. -->
          <span class="text-xs text-muted-foreground">
            The gap between one bell and the next. Most racks step 5 lb;
            adjustable dumbbells often do 2.5. A pair moves twice this, so yours
            go up {dumbbellStep * 2} lb at a time.
          </span>
        </label>

        <!-- The stacks. Three fields and not one, because they are three
             different facts that only look alike at the default: a
             selectorized stack usually steps 10 or 15, a cable stack is often
             5, and a band set jumps by whatever the manufacturer graded it.
             Before this screen existed they all silently took the bar's step,
             which is a sentence about plates and false about a pin. -->
        <fieldset class="flex flex-col gap-2">
          <legend class={labelClass}>Machines, cables and bands (lb)</legend>
          <p class="text-xs text-muted-foreground">
            The smallest jump each one makes. These are the whole weight, not
            per side — a pin goes in one stack. If you don't use something, the
            number costs you nothing.
          </p>
          <div class="mt-1 flex flex-col gap-2">
            <div class="flex items-center gap-3">
              <span
                class="w-24 text-sm font-bold text-card-foreground"
                id="machine-step-label"
              >
                Machines
              </span>
              <input
                bind:value={machineStep}
                type="number"
                min="1"
                max="50"
                step="0.5"
                required
                aria-labelledby="machine-step-label"
                class="{fieldClass} w-20"
              />
              <span class="text-xs text-muted-foreground">
                The gap between two holes. Usually 10 or 15.
              </span>
            </div>
            <div class="flex items-center gap-3">
              <span
                class="w-24 text-sm font-bold text-card-foreground"
                id="cable-step-label"
              >
                Cables
              </span>
              <input
                bind:value={cableStep}
                type="number"
                min="1"
                max="50"
                step="0.5"
                required
                aria-labelledby="cable-step-label"
                class="{fieldClass} w-20"
              />
              <span class="text-xs text-muted-foreground">
                The next plate up the stack. Usually 5.
              </span>
            </div>
            <div class="flex items-center gap-3">
              <span
                class="w-24 text-sm font-bold text-card-foreground"
                id="band-step-label"
              >
                Bands
              </span>
              <input
                bind:value={bandStep}
                type="number"
                min="1"
                max="50"
                step="0.5"
                required
                aria-labelledby="band-step-label"
                class="{fieldClass} w-20"
              />
              <span class="text-xs text-muted-foreground">
                The jump between one band and the next.
              </span>
            </div>
          </div>
        </fieldset>

        <fieldset class="flex flex-col gap-2">
          <legend class={labelClass}>Plates you own (pairs)</legend>
          <p class="text-xs text-muted-foreground">
            Pairs, not plates — one per side. Set a plate to 0 if you don't have
            any; the app won't ask you to load it.
          </p>
          <div class="mt-1 flex flex-col gap-2">
            {#each DENOMINATIONS as plateLb (plateLb)}
              <div class="flex items-center gap-3">
                <span
                  class="w-16 text-sm font-bold tabular-nums text-card-foreground"
                >
                  {plateLb} lb
                </span>
                <input
                  type="number"
                  min="0"
                  max="20"
                  value={pairsOf(plateLb)}
                  oninput={(e) =>
                    setPairs(plateLb, Number(e.currentTarget.value))}
                  aria-label={`Pairs of ${plateLb} lb plates`}
                  class="{fieldClass} w-20"
                />
              </div>
            {/each}
          </div>
        </fieldset>

        <div class="flex items-center gap-3">
          <Button type="submit" disabled={gymSaving}>
            {gymSaving ? "Saving…" : "Save"}
          </Button>
          {#if gymSaved}
            <span class="text-sm text-muted-foreground" role="status">Saved.</span>
          {/if}
        </div>
      </form>
    </Card>

    <Card class="p-6">
      <h3 class="text-lg font-bold text-card-foreground">Password</h3>
      <p class="mt-1 text-sm text-muted-foreground">
        Changing it signs out every other device.
      </p>
      <form class="mt-4 flex flex-col gap-4" onsubmit={savePassword}>
        {#if passwordError}
          <ErrorBanner
            message={passwordError}
            onDismiss={() => (passwordError = null)}
          />
        {/if}

        <label class="flex flex-col gap-1.5">
          <span class={labelClass}>Current password</span>
          <input
            bind:value={currentPassword}
            type="password"
            autocomplete="current-password"
            required
            class={fieldClass}
          />
        </label>

        <label class="flex flex-col gap-1.5">
          <span class={labelClass}>New password</span>
          <input
            bind:value={newPassword}
            type="password"
            autocomplete="new-password"
            required
            minlength="8"
            class={fieldClass}
          />
          <span class="text-xs text-muted-foreground">At least 8 characters.</span>
        </label>

        <div class="flex items-center gap-3">
          <Button type="submit" disabled={passwordSaving}>
            {passwordSaving ? "Changing…" : "Change password"}
          </Button>
          {#if passwordSaved}
            <span class="text-sm text-muted-foreground" role="status">
              Password changed.
            </span>
          {/if}
        </div>
      </form>
    </Card>

    <Card class="p-6">
      <h3 class="text-lg font-bold text-card-foreground">Your data</h3>
      <p class="mt-1 text-sm text-muted-foreground">
        Every session, set and setting on this account, as one JSON file. Yours
        to keep, and readable without this app.
      </p>
      <div class="mt-4 flex flex-col gap-3">
        {#if exportError}
          <ErrorBanner
            message={exportError}
            onDismiss={() => (exportError = null)}
          />
        {/if}
        <div>
          <Button type="button" onclick={exportAccount} disabled={exporting}>
            <Download class="size-4" aria-hidden="true" />
            {exporting ? "Preparing…" : "Download my data"}
          </Button>
        </div>
      </div>
    </Card>
  {/if}
</div>
