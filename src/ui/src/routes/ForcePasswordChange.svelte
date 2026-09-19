<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import { auth, loadMe } from "../lib/auth.svelte";
  import { changePassword } from "../lib/api";

  // What an account created by the admin sees, and the only thing it can see.
  //
  // The password it signed in with was typed by somebody else, who still knows
  // it. The API refuses every endpoint but /me and this one until it is
  // replaced (403 password_change_required), so this screen is not a nag that
  // can be dismissed — it is the whole app until it is satisfied. App.svelte
  // renders it INSTEAD of the router rather than on top of it, so there is no
  // route behind it to reach by typing a hash.
  //
  // Not a route of its own for the same reason: a URL is a thing you can
  // navigate away from.
  let currentPassword = $state("");
  let newPassword = $state("");
  let submitting = $state(false);
  let error = $state<string | null>(null);

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (submitting) return;
    submitting = true;
    error = null;

    const changed = await changePassword({ currentPassword, newPassword });
    if (changed.status !== 204) {
      // 401 is the temporary password being wrong, 400 is the new one failing
      // validation. Both are the user's to fix and both read the same way from
      // here.
      error = "Couldn't change your password. Check the one you were given and try again.";
      submitting = false;
      return;
    }

    // The server cleared the flag; /me is what tells this app so. Nothing to
    // navigate to — App.svelte swaps this screen for the router the moment
    // auth.me stops asking for a change.
    await loadMe();
    submitting = false;
  }
</script>

<div class="mx-auto w-full max-w-sm">
  <Card class="p-6">
    <h2 class="text-2xl font-black text-card-foreground">Set your password</h2>
    <p class="mt-1 text-sm text-muted-foreground">
      This account was created for you with a temporary password. Choose your own before
      you start — whoever set it up still knows the old one.
    </p>

    <form class="mt-5 flex flex-col gap-4" onsubmit={submit}>
      {#if error}
        <ErrorBanner message={error} onDismiss={() => (error = null)} />
      {/if}

      <!-- Hidden, and there for the browser's password manager rather than for
           the person: an autocomplete="new-password" field with no username in
           the form gives it nothing to file the new password under. -->
      <input
        type="text"
        name="username"
        autocomplete="username"
        value={auth.me?.username ?? ""}
        readonly
        hidden
      />

      <label class="flex flex-col gap-1.5">
        <span class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Temporary password
        </span>
        <input
          bind:value={currentPassword}
          type="password"
          autocomplete="current-password"
          required
          class="rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
        />
      </label>

      <label class="flex flex-col gap-1.5">
        <span class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          New password
        </span>
        <input
          bind:value={newPassword}
          type="password"
          autocomplete="new-password"
          required
          minlength="8"
          class="rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
        />
        <span class="text-xs text-muted-foreground">At least 8 characters.</span>
      </label>

      <Button type="submit" disabled={submitting}>
        {submitting ? "Saving…" : "Set password and continue"}
      </Button>
    </form>
  </Card>
</div>
