<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import Avatar from "../lib/Avatar.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import { auth, setMe } from "../lib/auth.svelte";
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

  async function saveProfile(event: SubmitEvent) {
    event.preventDefault();
    profileSaving = true;
    profileError = null;
    profileSaved = false;

    const { data, error } = await updateMe({ body: { displayName, avatarColor } });
    if (error || !data) {
      profileError = "Couldn't save your profile.";
    } else {
      setMe(data);
      profileSaved = true;
    }
    profileSaving = false;
  }

  async function savePassword(event: SubmitEvent) {
    event.preventDefault();
    passwordSaving = true;
    passwordError = null;
    passwordSaved = false;

    const { error } = await changePassword({
      body: { currentPassword, newPassword },
    });
    if (error) {
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
    const { data, error } = await uploadAvatar({ body: { avatar: file } });
    if (error || !data) {
      avatarError = "Couldn't upload that image. PNG or JPEG, up to 256 KB.";
    } else if (auth.me) {
      // Patch the etag locally so the <img> cache-buster changes and the new
      // picture appears at once, without a round trip to /me.
      setMe({ ...auth.me, hasAvatar: true, avatarEtag: data.etag });
    }
    avatarBusy = false;
    // Clear the input so re-picking the same file fires change again.
    input.value = "";
  }

  async function removeAvatar() {
    avatarBusy = true;
    avatarError = null;
    const { error } = await deleteAvatar();
    if (error) {
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
              {avatarBusy ? "Working…" : "Upload image"}
            </Button>
            {#if auth.me.hasAvatar}
              <Button
                type="button"
                variant="secondary"
                disabled={avatarBusy}
                onclick={removeAvatar}
              >
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
  {/if}
</div>
