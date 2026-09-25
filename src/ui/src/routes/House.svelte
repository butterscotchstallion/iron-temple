<script lang="ts">
  import { onMount } from "svelte";
  import { link, push } from "svelte-spa-router";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Avatar from "../lib/Avatar.svelte";
  import LifterName from "../lib/LifterName.svelte";
  import Timestamp from "../lib/Timestamp.svelte";
  import HouseIcon from "../lib/HouseIcon.svelte";
  import HouseForm from "../lib/HouseForm.svelte";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import Loading from "../lib/skeleton/Loading.svelte";
  import SkeletonRows from "../lib/skeleton/SkeletonRows.svelte";
  import { loadHouses } from "../lib/houses.svelte";
  import { pushToast } from "../lib/toast.svelte";
  import {
    getHouse,
    updateHouse,
    requestToJoinHouse,
    withdrawHouseRequest,
    approveHouseRequest,
    declineHouseRequest,
    leaveHouse,
    type CreateHouseRequest,
    type HouseDetail,
  } from "../lib/api";

  // One House: who it is, who is in it, and the way in.
  //
  // Fetches its own detail rather than reading houses.svelte.ts, because two of
  // the things on this page are not in that module and cannot be: the long
  // description, and the caller's own standing — whether they are a member, have a
  // request outstanding, or are in some other House. `viewer` is the server's
  // answer to that, and the buttons below read it rather than deducing it.
  //
  // Every write refreshes both this page AND the site-wide list, because joining
  // or leaving changes a sigil that is drawn on every other screen. Refreshing one
  // and not the other is how the tag beside a name goes stale for ten minutes.
  let { params }: { params: { id: string } } = $props();

  let house = $state<HouseDetail | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let notFound = $state(false);
  let editing = $state(false);
  let saving = $state(false);
  let formError = $state<string | null>(null);
  let busy = $state(false);

  const houseID = $derived(Number(params.id));

  // pendingRequests is ABSENT for anybody but the owner and EMPTY for an owner
  // with nobody waiting — see the field's note in the API. `?? null` keeps those
  // apart, so the section is drawn for an owner with an empty queue and not drawn
  // at all for a member.
  const pending = $derived(house?.pendingRequests ?? null);

  async function load() {
    failed = false;
    notFound = false;
    const result = await getHouse(houseID);
    if (result.status === 200) {
      house = result.data;
    } else if (result.status === 404) {
      notFound = true;
    } else {
      failed = true;
    }
    loading = false;
  }

  // Both, always, and in this order: the page the lifter is looking at first so it
  // stops lying soonest.
  async function refresh() {
    await load();
    await loadHouses();
  }

  async function save(body: CreateHouseRequest) {
    saving = true;
    formError = null;
    const result = await updateHouse(houseID, body);
    saving = false;

    if (result.status === 200) {
      house = result.data;
      editing = false;
      await loadHouses();
      pushToast({ title: "House saved", tone: "success" });
      return;
    }
    formError =
      result.status === 409
        ? "A House already has that name or sigil."
        : "That didn't work — check the name and sigil.";
  }

  // Asking has two outcomes and the status code is which. 201 filed a request
  // for an owner to answer; 200 means the House was standing empty and is now
  // the caller's, as its owner — see the endpoint's note in the API. Both
  // refresh, because either way the sigil beside their name elsewhere changed.
  async function ask() {
    busy = true;
    const result = await requestToJoinHouse(houseID);
    busy = false;
    if (result.status === 200) {
      await refresh();
      pushToast({
        title: "The House is yours",
        body: `Nobody was left in ${result.data.name}. You're its owner now.`,
        tone: "success",
      });
      return;
    }
    if (result.status === 201) {
      await refresh();
      pushToast({ title: "Asked to join", body: `${house?.name} will be told.` });
      return;
    }
    pushToast({ title: "Couldn't ask to join", tone: "danger" });
  }

  // Reads the id off state rather than taking it as an argument, because the
  // template's narrowing of `house` does not survive into an event handler — and
  // defaulting a missing id to 0 to satisfy that would send a request for a row
  // that cannot exist.
  async function withdrawOpen() {
    const requestID = house?.viewer.openRequestId;
    if (requestID === undefined) return;
    await withdraw(requestID);
  }

  async function withdraw(requestID: number) {
    busy = true;
    const result = await withdrawHouseRequest(houseID, requestID);
    busy = false;
    if (result.status === 204) {
      await refresh();
      return;
    }
    pushToast({ title: "Couldn't withdraw", tone: "danger" });
  }

  async function decide(requestID: number, approve: boolean) {
    busy = true;
    const result = approve
      ? await approveHouseRequest(houseID, requestID)
      : await declineHouseRequest(houseID, requestID);
    busy = false;
    if (result.status === 204) {
      await refresh();
      return;
    }
    // 409 is the one worth naming: the lifter joined somewhere else while the
    // request sat here, so there is nothing wrong with the House or the tap.
    pushToast({
      title: result.status === 409 ? "They've joined another House" : "That didn't work",
      tone: "danger",
    });
  }

  async function leave() {
    busy = true;
    const result = await leaveHouse();
    busy = false;
    if (result.status === 204) {
      await loadHouses();
      pushToast({ title: "You left the House" });
      void push("/houses");
      return;
    }
    pushToast({ title: "Couldn't leave", tone: "danger" });
  }

  onMount(load);
</script>

<div class="flex flex-col gap-4">
  <a
    href="/houses"
    use:link
    class="text-sm text-muted-foreground transition hover:text-neon-lift focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
  >
    ← All Houses
  </a>

  {#if loading}
    <Card class="p-6">
      <Loading label="Loading the House">
        <SkeletonRows rows={3} avatar="size-10" />
      </Loading>
    </Card>
  {:else if notFound}
    <Card class="p-6">
      <!-- No longer blames an empty House: one stands after its last member
           leaves, so a 404 here really is "there was never a House with this
           id", not "you just missed it". -->
      <p class="text-sm text-muted-foreground">That House doesn't exist.</p>
    </Card>
  {:else if failed || !house}
    <ErrorCard message="Couldn't load this House." onRetry={load} />
  {:else}
    <Card class="p-6">
      <div class="flex items-start gap-4">
        <HouseIcon {house} size="size-10" />
        <div class="flex min-w-0 flex-col gap-1">
          <div class="flex flex-wrap items-baseline gap-2">
            <h2 class="text-2xl font-black text-foreground">{house.name}</h2>
            <span
              class="rounded-sm bg-primary/15 px-1.5 py-0.5 text-xs font-semibold tracking-wide text-primary uppercase"
            >
              {house.sigil}
            </span>
          </div>
          {#if house.tagline}
            <p class="text-sm text-muted-foreground">{house.tagline}</p>
          {/if}
          <p class="text-xs text-muted-foreground">
            {house.memberCount === 1 ? "1 member" : `${house.memberCount} members`}
            · Founded <Timestamp value={house.createdAt} />
          </p>
        </div>
      </div>

      {#if house.description}
        <!-- whitespace-pre-line so paragraphs a lifter typed survive. The field is
             prose, not markup, and rendering it as one run would silently join
             their lines together. -->
        <p class="mt-4 text-sm whitespace-pre-line text-foreground/90">
          {house.description}
        </p>
      {/if}

      <div class="mt-5 flex flex-wrap items-center gap-3">
        {#if house.viewer.isMember}
          {#if house.viewer.isOwner && !editing}
            <Button variant="outline" size="sm" onclick={() => (editing = true)}>
              <Pencil />
              Edit
            </Button>
          {/if}
          <Button variant="destructive" size="sm" onclick={leave} disabled={busy}>
            Leave House
          </Button>
        {:else if house.viewer.openRequestId !== undefined}
          <span class="text-sm text-muted-foreground">You've asked to join.</span>
          <Button
            variant="outline"
            size="sm"
            onclick={withdrawOpen}
            disabled={busy}
          >
            Withdraw
          </Button>
        {:else if house.viewer.inAnotherHouse}
          <!-- Said rather than shown as a disabled button: a lifter who cannot
               ask should be told which rule stopped them, not left tapping. -->
          <span class="text-sm text-muted-foreground">
            You're already in a House. Leave it first to ask to join this one.
          </span>
        {:else}
          <Button onclick={ask} disabled={busy}>Request to join</Button>
        {/if}
      </div>

      {#if editing && house.viewer.isOwner}
        <div class="mt-4 border-t border-border/60 pt-4">
          <HouseForm
            initial={{
              name: house.name,
              sigil: house.sigil,
              tagline: house.tagline,
              description: house.description,
              icon: house.icon,
              iconColor: house.iconColor,
            }}
            {saving}
            error={formError}
            submitLabel="Save"
            onsubmit={save}
            oncancel={() => {
              editing = false;
              formError = null;
            }}
          />
        </div>
      {/if}
    </Card>

    {#if pending}
      <Card class="p-6">
        <h3 class="text-lg font-bold text-foreground">Asking to join</h3>
        {#if pending.length === 0}
          <p class="mt-2 text-sm text-muted-foreground">Nobody is waiting.</p>
        {:else}
          <ul class="mt-2 flex flex-col divide-y divide-border/60">
            {#each pending as request (request.id)}
              <li class="flex items-center gap-3 py-3">
                <Avatar user={request.lifter} size={36} />
                <span class="flex min-w-0 flex-1 flex-col">
                  <LifterName lifter={request.lifter} class="font-semibold text-foreground" />
                  <span class="text-xs text-muted-foreground">
                    Asked <Timestamp value={request.requestedAt} />
                  </span>
                </span>
                <span class="flex shrink-0 gap-2">
                  <Button size="sm" onclick={() => decide(request.id, true)} disabled={busy}>
                    Approve
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onclick={() => decide(request.id, false)}
                    disabled={busy}
                  >
                    Decline
                  </Button>
                </span>
              </li>
            {/each}
          </ul>
        {/if}
      </Card>
    {/if}

    <Card class="p-6">
      <h3 class="text-lg font-bold text-foreground">Members</h3>
      <p class="mt-1 text-sm text-muted-foreground">
        What everyone here is wearing. Crowns come from the leaderboard.
      </p>
      <ul class="mt-3 flex flex-col divide-y divide-border/60">
        {#each house.members as member (member.lifter.id)}
          <li>
            <a
              href={`/lifters/${member.lifter.id}`}
              use:link
              class="-mx-2 flex items-center gap-3 rounded-md px-2 py-3 transition hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
              <Avatar user={member.lifter} size={40} />
              <span class="flex min-w-0 flex-col">
                <span class="flex items-baseline gap-2">
                  <!-- sigil={false}: every member of this House wears the same
                       one, and the page's heading already said which. -->
                  <LifterName
                    lifter={member.lifter}
                    class="font-semibold text-foreground"
                    sigil={false}
                  />
                  {#if member.isOwner}
                    <span class="shrink-0 text-xs text-muted-foreground">(owner)</span>
                  {/if}
                </span>
                <span class="truncate text-sm text-muted-foreground">
                  {#if member.lifter.lastTrainedOn}
                    Last trained <Timestamp
                      value={member.lifter.lastTrainedOn}
                      kind="date"
                    />
                  {:else}
                    Hasn't trained yet
                  {/if}
                </span>
              </span>
            </a>
          </li>
        {/each}
      </ul>
    </Card>
  {/if}
</div>
