<script lang="ts">
  import Loading from "./Loading.svelte";
  import Skeleton from "./Skeleton.svelte";

  // Test-only harness. Loading takes a snippet, which a .ts test file cannot
  // build — so the children live in a real component here. See Loading.test.ts.
  let {
    label = "Loading your history",
    class: className,
  }: {
    label?: string;
    class?: string;
  } = $props();
</script>

<Loading {label} class={className}>
  <Skeleton text="2xl" class="w-40" />
</Loading>
