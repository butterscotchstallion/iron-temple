<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import { auth, signIn, signUp } from "../lib/auth.svelte";

  // One form for both signing in and claiming the install. Which one is on
  // offer is the server's call (`/auth/registration-status`), not a route or a
  // toggle: registration closes permanently after the first account, so a
  // "create account" tab that always 403s would be a dead end.
  let username = $state("");
  let displayName = $state("");
  let password = $state("");
  let rememberMe = $state(true);
  let submitting = $state(false);
  let error = $state<string | null>(null);

  let registering = $derived(auth.registrationOpen);

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (submitting) return;
    submitting = true;
    error = null;

    error = registering
      ? await signUp(username, displayName, password, rememberMe)
      : await signIn(username, password, rememberMe);

    submitting = false;
    // On success the app re-renders on `auth.me`; nothing to navigate to.
  }
</script>

<div class="mx-auto w-full max-w-sm">
  <Card class="p-6">
    <h2 class="text-2xl font-black text-card-foreground">
      {registering ? "Claim this install" : "Sign in"}
    </h2>
    <p class="mt-1 text-sm text-muted-foreground">
      {registering
        ? "No account exists yet. The first one you create owns this install — registration closes afterwards."
        : "Sign in to see your workouts."}
    </p>

    <form class="mt-5 flex flex-col gap-4" onsubmit={submit}>
      {#if error}
        <ErrorBanner message={error} onDismiss={() => (error = null)} />
      {/if}

      <label class="flex flex-col gap-1.5">
        <span class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Username
        </span>
        <input
          bind:value={username}
          name="username"
          autocomplete="username"
          required
          minlength="3"
          maxlength="32"
          class="rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
        />
      </label>

      {#if registering}
        <label class="flex flex-col gap-1.5">
          <span class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
            Display name
          </span>
          <input
            bind:value={displayName}
            name="displayName"
            autocomplete="name"
            maxlength="64"
            placeholder="Optional — defaults to your username"
            class="rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
          />
        </label>
      {/if}

      <label class="flex flex-col gap-1.5">
        <span class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Password
        </span>
        <input
          bind:value={password}
          name="password"
          type="password"
          autocomplete={registering ? "new-password" : "current-password"}
          required
          minlength={registering ? 8 : undefined}
          class="rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
        />
        {#if registering}
          <span class="text-xs text-muted-foreground">At least 8 characters.</span>
        {/if}
      </label>

      <label class="flex cursor-pointer items-center gap-2 text-sm text-muted-foreground">
        <input
          bind:checked={rememberMe}
          type="checkbox"
          name="rememberMe"
          class="size-4 rounded border-border/60 accent-primary"
        />
        Remember me
      </label>

      <Button type="submit" disabled={submitting}>
        {#if submitting}
          Working…
        {:else}
          {registering ? "Create account" : "Sign in"}
        {/if}
      </Button>
    </form>
  </Card>
</div>
