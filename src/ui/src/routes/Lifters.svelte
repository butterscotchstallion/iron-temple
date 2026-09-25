<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { Card } from "$lib/components/ui/card";
  import Avatar from "../lib/Avatar.svelte";
  import LifterName from "../lib/LifterName.svelte";
  import FollowButton from "../lib/FollowButton.svelte";
  import Timestamp from "../lib/Timestamp.svelte";
  import { auth } from "../lib/auth.svelte";
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
          <!-- The row is a flex container with the link as its growing child,
               rather than one anchor wrapping everything. A <button> inside an
               <a> is invalid markup and its click bubbles into the navigation,
               so the Follow control has to be the link's sibling.

               The highlight therefore belongs to the row, not to the link: on the
               link it stopped where the link stopped, leaving the Follow control
               sitting in an unlit strip at the end of a lit row. The `-mx-2 px-2`
               that bleeds it past the avatar moves here with it, and the focus
               ring follows via `has-[a:focus-visible]` so arriving by keyboard
               lights the same shape the mouse does. The link keeps `py-3`, which
               is what makes the row's full height clickable. -->
          <li
            class="-mx-2 flex items-center gap-2 rounded-md px-2 transition hover:bg-white/5 has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-primary"
          >
            <a
              href={`/lifters/${lifter.id}`}
              use:link
              class="flex min-w-0 flex-1 items-center gap-3 py-3 focus-visible:outline-none"
            >
              <Avatar user={lifter} size={40} />
              <span class="flex min-w-0 flex-col">
                <span class="flex items-baseline gap-2">
                  <LifterName {lifter} class="font-semibold text-foreground" />
                  {#if lifter.id === auth.me?.id}
                    <span class="text-xs text-muted-foreground">(you)</span>
                  {/if}
                </span>
                <!-- "Hasn't trained yet" rather than a date, because the field is
                     absent for an account that has never logged a rep — see the
                     Lifter schema. An account created this morning and an account
                     that lifted for a year and stopped are different facts, and
                     only one of them has a date to show. -->
                <span class="truncate text-sm text-muted-foreground">
                  {#if lifter.lastTrainedOn}
                    Last trained <Timestamp
                      value={lifter.lastTrainedOn}
                      kind="date"
                    />
                  {:else}
                    Hasn't trained yet
                  {/if}
                </span>
              </span>
            </a>
            <!-- Not on your own row. You cannot follow yourself and your own
                 achievements already reach you, so the control would refuse
                 every press — the same call SessionSocial makes about applauding
                 your own workout. `following` is absent on a Lifter that did not
                 come from this endpoint, so `=== true` rather than a truthiness
                 test that would read undefined as a deliberate false. -->
            {#if lifter.id !== auth.me?.id}
              <FollowButton
                {lifter}
                size="sm"
                following={lifter.following === true}
                onChange={(next) => (lifter.following = next)}
              />
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </Card>
</div>
