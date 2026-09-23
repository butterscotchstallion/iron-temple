<script lang="ts">
  import { replace } from "svelte-spa-router";
  import ProfileNav from "../lib/profile/ProfileNav.svelte";
  import AvatarSection from "../lib/profile/AvatarSection.svelte";
  import DetailsSection from "../lib/profile/DetailsSection.svelte";
  import PasswordSection from "../lib/profile/PasswordSection.svelte";
  import EquipmentSection from "../lib/profile/EquipmentSection.svelte";
  import DataSection from "../lib/profile/DataSection.svelte";
  import { DEFAULT_SECTION, toSection } from "../lib/profile/sections";
  import { auth } from "../lib/auth.svelte";

  // The profile is five unrelated settings screens that happen to belong to one
  // account. They used to be five cards stacked on one route, which meant
  // scrolling past the plate rack to change a password and no way to link
  // anybody to the part you meant.
  //
  // So: one section per URL (`#/profile/equipment`), a pill row to move between
  // them, and one component per section. Each keeps its own draft state and its
  // own Save — which is also what unmounting buys, since leaving a section is
  // now how an unwanted edit gets discarded.
  //
  // `params.section` is optional because both `/profile` and `/profile/:section`
  // route here; a bare `/profile` is not a sixth state, it redirects.
  let { params = {} }: { params?: { section?: string } } = $props();

  // null for a bare /profile and for a slug that names nothing — both of which
  // the effect below turns into a real URL rather than rendering.
  const section = $derived(toSection(params.section));

  // replace() rather than push(): landing on /profile and then pressing Back
  // should leave the profile, not bounce between the bare path and the section
  // it sent you to. The same goes for a mistyped slug — there is nothing there
  // worth a history entry.
  $effect(() => {
    if (section === null) void replace(`/profile/${DEFAULT_SECTION}`);
  });
</script>

<div class="flex flex-col gap-6">
  <h2 class="text-2xl font-black text-foreground">Profile</h2>

  <!-- Both conditions matter. `auth.me` is null until /me settles, and every
       section is built out of what it returned; `section` is null for the beat
       between a bare /profile mounting and the redirect above taking effect. -->
  {#if auth.me && section}
    <ProfileNav current={section} />

    {#if section === "avatar"}
      <AvatarSection me={auth.me} />
    {:else if section === "details"}
      <DetailsSection me={auth.me} />
    {:else if section === "password"}
      <PasswordSection />
    {:else if section === "equipment"}
      <EquipmentSection me={auth.me} />
    {:else if section === "data"}
      <DataSection />
    {/if}
  {/if}
</div>
