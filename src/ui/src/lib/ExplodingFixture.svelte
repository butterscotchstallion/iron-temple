<script lang="ts">
  import { untrack } from "svelte";

  // Test-only. Throws while rendering, the way a route with a genuine bug would.
  let { shouldThrow = true }: { shouldThrow?: boolean } = $props();

  // Reading the initial value is the whole point — the throw has to happen during
  // the first render. untrack() says so, instead of tripping state_referenced_locally.
  if (untrack(() => shouldThrow)) {
    throw new Error("kaboom");
  }
</script>

<p>the route rendered fine</p>
