<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import ErrorBanner from "../ErrorBanner.svelte";
  import { changePassword } from "../api";
  import { fieldClass, labelClass } from "./fieldStyles";

  let currentPassword = $state("");
  let newPassword = $state("");
  let passwordSaving = $state(false);
  let passwordError = $state<string | null>(null);
  let passwordSaved = $state(false);

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
</script>

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
