<script lang="ts">
  import { onMount } from "svelte";
  import { link, push } from "svelte-spa-router";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import Plus from "@lucide/svelte/icons/plus";
  import HouseIcon from "../lib/HouseIcon.svelte";
  import HouseForm from "../lib/HouseForm.svelte";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import Loading from "../lib/skeleton/Loading.svelte";
  import SkeletonRows from "../lib/skeleton/SkeletonRows.svelte";
  import { auth } from "../lib/auth.svelte";
  import { houses, houseFor, loadHouses } from "../lib/houses.svelte";
  import { createHouse, type CreateHouseRequest } from "../lib/api";
  import { pushToast } from "../lib/toast.svelte";

  // Every House on the install, and the way into founding one.
  //
  // Reads the same module every sigil reads rather than fetching its own list.
  // That is not only thrift: this screen and the tag beside a name must not be
  // able to disagree about which Houses exist, and one source is how that is
  // guaranteed rather than hoped for.
  //
  // It still calls loadHouses on mount. The poller in App.svelte runs every ten
  // minutes, which is right for an ornament and much too slow for the screen
  // somebody just opened in order to look at it.
  let failed = $state(false);
  let founding = $state(false);
  let saving = $state(false);
  let formError = $state<string | null>(null);

  const mine = $derived(houseFor(auth.me?.id));
  const loading = $derived(!houses.loaded && !failed);

  async function load() {
    failed = false;
    await loadHouses();
    // loadHouses is deliberately silent on failure — see its note — so the only
    // way to tell a failed first load from an install with no Houses is that the
    // module never became `loaded`.
    failed = !houses.loaded;
  }

  async function found(body: CreateHouseRequest) {
    saving = true;
    formError = null;
    const result = await createHouse(body);
    saving = false;

    if (result.status === 201) {
      await loadHouses();
      founding = false;
      pushToast({ title: `${result.data.name} founded`, tone: "success" });
      void push(`/houses/${result.data.id}`);
      return;
    }
    // A 409 says the name or the sigil is taken and a 400 says one of them is
    // malformed. Both are things the lifter can fix in the form they are looking
    // at, so they are shown there rather than as a toast that outlives the fix.
    formError =
      result.status === 409
        ? "A House already has that name or sigil."
        : "That didn't work — check the name and sigil.";
  }

  onMount(load);
</script>

<div class="flex flex-col gap-4">
  <div class="flex items-start justify-between gap-3">
    <div>
      <h2 class="text-2xl font-black text-foreground">Houses</h2>
      <p class="mt-1 text-sm text-muted-foreground">
        A House is a group of lifters who train together. Members wear its sigil
        beside their name.
      </p>
    </div>
    {#if !mine && !founding}
      <Button variant="outline" size="sm" onclick={() => (founding = true)}>
        <Plus />
        Found one
      </Button>
    {/if}
  </div>

  {#if founding}
    <Card class="p-6">
      <h3 class="text-lg font-bold text-foreground">Found a House</h3>
      <HouseForm
        {saving}
        error={formError}
        submitLabel="Found it"
        onsubmit={found}
        oncancel={() => {
          founding = false;
          formError = null;
        }}
      />
    </Card>
  {/if}

  {#if mine}
    <!-- Named before the list rather than only marked within it. A lifter is in
         at most one House and it is the one thing they came here to find. -->
    <Card class="p-6">
      <p class="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
        Your House
      </p>
      <a
        href={`/houses/${mine.id}`}
        use:link
        class="-mx-2 mt-2 flex items-center gap-3 rounded-md px-2 py-2 transition hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      >
        <HouseIcon house={mine} size="size-8" />
        <span class="flex min-w-0 flex-col">
          <span class="truncate font-semibold text-foreground">{mine.name}</span>
          <span class="truncate text-sm text-muted-foreground">
            {mine.tagline || mine.sigil}
          </span>
        </span>
      </a>
    </Card>
  {/if}

  <Card class="p-6">
    {#if loading}
      <Loading label="Loading the Houses">
        <SkeletonRows rows={2} avatar="size-10" />
      </Loading>
    {:else if failed}
      <ErrorCard message="Couldn't load the Houses." onRetry={load} />
    {:else if houses.items.length === 0}
      <p class="text-sm text-muted-foreground">
        Nobody has founded a House yet. Found the first one.
      </p>
    {:else}
      <ul class="flex flex-col divide-y divide-border/60">
        {#each houses.items as house (house.id)}
          <li>
            <a
              href={`/houses/${house.id}`}
              use:link
              class="-mx-2 flex items-center gap-3 rounded-md px-2 py-3 transition hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
              <HouseIcon {house} size="size-8" />
              <span class="flex min-w-0 flex-col">
                <span class="flex items-baseline gap-2">
                  <span class="truncate font-semibold text-foreground">{house.name}</span>
                  <span
                    class="shrink-0 rounded-sm bg-primary/15 px-1 py-px text-[0.65rem] font-semibold tracking-wide text-primary uppercase"
                  >
                    {house.sigil}
                  </span>
                </span>
                <span class="truncate text-sm text-muted-foreground">
                  {house.tagline ||
                    (house.memberCount === 1 ? "1 member" : `${house.memberCount} members`)}
                </span>
              </span>
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  </Card>
</div>
