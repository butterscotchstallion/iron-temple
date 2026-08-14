<script lang="ts">
  import ErrorBoundary from "./ErrorBoundary.svelte";
  import Exploding from "./ExplodingFixture.svelte";

  // Test-only harness. ErrorBoundary takes a snippet, and a snippet that throws
  // during render can't be built from a .ts test file — so the throw lives in a
  // real child component here. See ErrorBoundary.test.ts.
  let { shouldThrow = true }: { shouldThrow?: boolean } = $props();
</script>

<ErrorBoundary>
  <Exploding {shouldThrow} />
</ErrorBoundary>
