<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { Card } from "$lib/components/ui/card";
  import Avatar from "../lib/Avatar.svelte";
  import LifterName from "../lib/LifterName.svelte";
  import { auth } from "../lib/auth.svelte";
  import { formatLongDate } from "../lib/date";
  import { listLifters, type Lifter } from "../lib/api";
  import Loading from "../lib/skeleton/Loading.svelte";
  import SkeletonRows from "../lib/skeleton/SkeletonRows.svelte";

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
      <!-- This used to render nothing on the grounds that one local request is
           a short beat. It is — but "nothing" is a card that collapses to its
           own padding and then grows by a row per lifter, and it also leaves a
           screen reader with an empty page rather than a "loading" to wait on.
           Two rows at the roster's own 40px avatar, so the list fills in rather
           than unfolding. Two rather than more because this install may well
           hold one lifter, and over-reserving is the same jump upside down. -->
      <Loading label="Loading the lifters">
        <SkeletonRows rows={2} avatar="size-10" />
      </Loading>
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
                  <LifterName {lifter} class="font-semibold text-foreground" />
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
