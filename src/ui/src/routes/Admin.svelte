<script lang="ts">
  import { onMount } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import UserPlus from "@lucide/svelte/icons/user-plus";
  import Dices from "@lucide/svelte/icons/dices";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import { auth } from "../lib/auth.svelte";
  import { formatLongDate } from "../lib/date";
  import { passphrase } from "../lib/passphrase";
  import { randomPunName } from "../lib/punNames";
  import { createUser, listUsers, type AdminUser } from "../lib/api";

  // Who has an account on this install, and how to add one.
  //
  // Reachable only by the account that claimed the install. The route condition
  // in App.svelte keeps the link and the hash from working for anyone else, but
  // that is convenience — the API answers 403 admin_required regardless, and
  // that is the check that matters.
  //
  // Deliberately NOT cached through cache.svelte, unlike every other list in
  // the app. That cache exists because the four training tabs are revisited
  // constantly and a stale-then-corrected paint beats a skeleton; this screen is
  // opened once in a while to do one thing, and an admin who has just added
  // somebody must never be shown a roster that predates them.
  let users = $state<AdminUser[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  // The username starts filled in with a lifting pun, and the dice beside the
  // field rolls another. Naming an account is the one part of this form that
  // has no right answer, and a box that already says something is easier to
  // argue with than an empty one.
  //
  // `suggested` is what the roll last put there. Comparing the field against it
  // is how the code tells "the admin has left the suggestion alone" from "the
  // admin typed something", without an input handler that would have to
  // duplicate what bind:value already does.
  const firstSuggestion = randomPunName();
  let suggested = $state(firstSuggestion);
  let username = $state(firstSuggestion);
  let displayName = $state("");
  // Filled in before the admin gets here, not left blank for them to invent
  // something. Asking one person to choose a password for another reliably
  // produces the lifter's own first name; a generated passphrase makes the good
  // answer the default and the bad one extra work. Still an ordinary editable
  // field — see passphrase.ts for why words rather than characters.
  let password = $state(passphrase());
  let creating = $state(false);
  let createError = $state<string | null>(null);
  let created = $state<string | null>(null);

  // Roll a name the roster doesn't already have, and one that isn't the name on
  // screen — a button that can appear to do nothing reads as a broken button.
  function suggest() {
    suggested = randomPunName([username, ...users.map((user) => user.username)]);
    username = suggested;
  }

  async function load() {
    failed = false;
    const result = await listUsers();
    if (result.status !== 200) {
      failed = true;
    } else {
      users = result.data;
    }
    loading = false;

    // The first suggestion is made before the roster arrives, so it is the one
    // roll that can't check itself against it. Re-roll if it turns out to be
    // taken — but only while it is still untouched, because the admin typing
    // over it outranks anything this can offer.
    const collides = users.some(
      (user) => user.username.toLowerCase() === suggested.toLowerCase(),
    );
    if (username === suggested && collides) {
      suggest();
    }
  }

  async function add(event: SubmitEvent) {
    event.preventDefault();
    if (creating) return;

    creating = true;
    createError = null;
    created = null;

    const result = await createUser({
      username: username.trim(),
      displayName: displayName.trim(),
      password,
    });
    creating = false;

    if (result.status !== 201) {
      // The server names the reason — a taken username, a password too short —
      // and that is more use than "couldn't create the account".
      createError = result.data?.message ?? "Couldn't create that account.";
      return;
    }

    // Append rather than refetch: the roster is oldest-first, so a new account
    // belongs at the end and one insertion is cheaper and less jarring than
    // reloading the whole list.
    users = [...users, result.data];
    created = result.data.username;
    // Reset to a fresh suggestion rather than to an empty field: the roster now
    // includes the account just made, so the next name is guaranteed to differ
    // from it.
    suggest();
    displayName = "";
    // A fresh one rather than an empty field, so adding two people in a row is
    // the same two keystrokes as adding one — and so the second account never
    // reuses the password just read down the phone for the first.
    password = passphrase();
  }

  // createdAt is an RFC 3339 instant; formatLongDate takes a date-only string.
  // The day is all this column shows, and the leading ten characters are it.
  function createdOn(iso: string): string {
    return formatLongDate(iso.slice(0, 10));
  }

  onMount(load);
</script>

<div class="flex flex-col gap-4">
  <div>
    <h2 class="text-2xl font-black text-foreground">Accounts</h2>
    <p class="mt-1 text-sm text-muted-foreground">
      Everyone who can sign in to this install. Sign-up closed when you claimed it, so
      this is the only way to add someone.
    </p>
  </div>

  <Card class="p-6">
    <h3 class="text-lg font-bold text-card-foreground">Add someone</h3>
    <p class="mt-1 text-sm text-muted-foreground">
      Give them the password you set here out of band. They'll be asked to replace it
      before they can use the app, and until they do, it's the only thing their account
      can do.
    </p>

    <form class="mt-4 flex flex-col gap-4" onsubmit={add}>
      {#if createError}
        <ErrorBanner message={createError} onDismiss={() => (createError = null)} />
      {/if}

      <!-- The only field here whose label is explicit rather than wrapping its
           input: the dice is a control of its own, and nesting a button inside
           a <label> makes a click on it ambiguous. -->
      <div class="flex flex-col gap-1.5">
        <label
          for="admin-username"
          class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
        >
          Username
        </label>
        <div class="flex items-center gap-2">
          <input
            bind:value={username}
            id="admin-username"
            name="username"
            autocomplete="off"
            required
            minlength="3"
            maxlength="32"
            class="min-w-0 flex-1 rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
          />
          <Button
            type="button"
            variant="outline"
            size="icon"
            onclick={suggest}
            title="Suggest another name"
            aria-label="Suggest another name"
          >
            <Dices class="size-4" aria-hidden="true" />
          </Button>
        </div>
        <span class="text-xs text-muted-foreground">
          Letters, digits, dot, underscore and hyphen.
        </span>
      </div>

      <label class="flex flex-col gap-1.5">
        <span class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Display name
        </span>
        <input
          bind:value={displayName}
          name="displayName"
          autocomplete="off"
          maxlength="64"
          placeholder="Optional — defaults to the username"
          class="rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
        />
      </label>

      <label class="flex flex-col gap-1.5">
        <span class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          Temporary password
        </span>
        <!-- type="text": the admin has to read this back to the person it is
             for, and a field of dots they cannot check is how a typo becomes an
             account nobody can sign in to. It is a one-time credential on a
             screen only the owner can open. Doubly so now it is generated —
             a masked field of words nobody chose would be unreadable AND
             unmemorable.

             autocomplete="off" and not "new-password": the browser offering to
             save this would file somebody else's one-time credential under the
             admin's own login. -->
        <input
          bind:value={password}
          name="password"
          type="text"
          autocomplete="off"
          required
          minlength="8"
          class="rounded-md border border-border/60 bg-input/40 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
        />
        <span class="text-xs text-muted-foreground">
          Four random words, generated in your browser. Read it to them as it is, or
          type over it — at least 8 characters.
        </span>
      </label>

      <div class="flex items-center gap-3">
        <Button type="submit" disabled={creating}>
          <UserPlus class="size-4" aria-hidden="true" />
          {creating ? "Creating…" : "Create account"}
        </Button>
        {#if created}
          <span class="text-sm text-muted-foreground" role="status">
            Created {created}.
          </span>
        {/if}
      </div>
    </form>
  </Card>

  <Card class="p-6">
    <h3 class="text-lg font-bold text-card-foreground">
      {users.length === 1 ? "1 account" : `${users.length} accounts`}
    </h3>

    {#if loading}
      <!-- Nothing, for the beat it takes: one local request, and a flash of
           skeleton for a short table is worse than the space it will fill. -->
    {:else if failed}
      <p class="mt-3 text-sm text-muted-foreground">
        Couldn't load the accounts. Reload to try again.
      </p>
    {:else}
      <ul class="mt-3 flex flex-col divide-y divide-border/60">
        {#each users as user (user.id)}
          <li class="flex flex-wrap items-baseline gap-x-3 gap-y-1 py-3">
            <span class="font-semibold text-foreground">
              {user.displayName || user.username}
            </span>
            <span class="text-sm text-muted-foreground">{user.username}</span>

            {#if user.isAdmin}
              <span
                class="rounded-full bg-primary/15 px-2 py-0.5 text-xs font-semibold uppercase tracking-[0.15em] text-primary"
              >
                Owner
              </span>
            {/if}
            {#if user.id === auth.me?.id}
              <span class="text-xs text-muted-foreground">(you)</span>
            {/if}
            {#if user.mustChangePassword}
              <!-- Worth surfacing: the temporary password is still live, and
                   still known to whoever typed it. -->
              <span
                class="rounded-full bg-white/10 px-2 py-0.5 text-xs font-semibold uppercase tracking-[0.15em] text-muted-foreground"
              >
                Hasn't set a password
              </span>
            {/if}

            <span class="ml-auto text-sm text-muted-foreground">
              {createdOn(user.createdAt)}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  </Card>
</div>
