<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { Card } from "$lib/components/ui/card";
  import Avatar from "../lib/Avatar.svelte";
  import { auth } from "../lib/auth.svelte";
  import { formatLongDate } from "../lib/date";
  import { listLifters, type Lifter } from "../lib/api";

  // Everyone who trains on this install.
  //
  // The app was built for one lifter and says so; this is the screen that admits
  // there may be more than one. It is reached from the account menu rather than
  // the nav bar, which is full — five tabs already wrap onto two rows on a phone
  // — and which is anyway the wrong place for it: the nav is the four or five
  // screens you move between while training, and this is not one of them.
  //
  // Not cached through cache.svelte, for the reason Admin gives: that cache pays
  // for itself on the training tabs, which are revisited constantly. This is a
  // page you open now and then.
  let lifters = $state<Lifter[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  async function load() {
    failed = false;
    const result = await listLifters();
    if (result.status !== 200) {
      failed = true;
    } else {
      lifters = result.data;
    }
    loading = false;
  }

  // "Has not trained yet" rather than a date, because the field is absent for an
  // account that has never logged a rep — see the Lifter schema. An account
  // created this morning and an account that lifted for a year and stopped are
  // different facts, and only one of them has a date to show.
  function lastTrained(lifter: Lifter): string {
    if (!lifter.lastTrainedOn) return "Hasn't trained yet";
    return `Last trained ${formatLongDate(lifter.lastTrainedOn)}`;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-4">
  <div>
    <h2 class="text-2xl font-black text-foreground">Lifters</h2>
    <p class="mt-1 text-sm text-muted-foreground">
      Everyone training on this install. Open one to see what they've been lifting.
    </p>
  </div>

  <Card class="p-6">
    {#if loading}
      <!-- Nothing for the beat it takes: one local request, and a skeleton for a
           short list is worse than the space it will fill. Same call Admin makes. -->
    {:else if failed}
      <p class="text-sm text-muted-foreground">
        Couldn't load the lifters. Reload to try again.
      </p>
    {:else}
      <ul class="flex flex-col divide-y divide-border/60">
        {#each lifters as lifter (lifter.id)}
          <li>
            <a
              href={`/lifters/${lifter.id}`}
              use:link
              class="-mx-2 flex items-center gap-3 rounded-md px-2 py-3 transition hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
              <Avatar user={lifter} size={40} />
              <span class="flex min-w-0 flex-col">
                <span class="flex items-baseline gap-2">
                  <span class="truncate font-semibold text-foreground">
                    {lifter.displayName || lifter.username}
                  </span>
                  {#if lifter.id === auth.me?.id}
                    <span class="text-xs text-muted-foreground">(you)</span>
                  {/if}
                </span>
                <span class="truncate text-sm text-muted-foreground">
                  {lastTrained(lifter)}
                </span>
              </span>
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  </Card>
</div>
