<script lang="ts">
  import { onMount } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import FeedList from "../lib/FeedList.svelte";
  import Loading from "../lib/skeleton/Loading.svelte";
  import SkeletonRows from "../lib/skeleton/SkeletonRows.svelte";
  import { getFeed, type FeedEntry } from "../lib/api";

  // Everything the other lifters have logged, newest first.
  //
  // The endpoint sends no total, so "is there more" is answered by the page
  // length: a full page might have more behind it, a short one is the end. That
  // is the whole reason PAGE is a constant here rather than a number typed into
  // the call — the comparison below has to be against the size actually asked
  // for.
  const PAGE = 20;

  let items = $state<FeedEntry[]>([]);
  let loading = $state(true);
  let failed = $state(false);
  let loadingMore = $state(false);
  // Starts true so the first load is allowed to run; set from each response.
  let more = $state(true);

  async function loadPage(offset: number) {
    const result = await getFeed({ limit: PAGE, offset });
    if (result.status !== 200) return false;

    // Appended rather than replaced, so paging accumulates. Concatenating is
    // safe against duplicates here in a way it would not be under keyset paging:
    // the offset advances by exactly what arrived.
    items = offset === 0 ? result.data.items : [...items, ...result.data.items];
    more = result.data.items.length === PAGE;
    return true;
  }

  async function load() {
    failed = false;
    loading = true;
    if (!(await loadPage(0))) failed = true;
    loading = false;
  }

  async function loadMore() {
    if (loadingMore || !more) return;
    loadingMore = true;
    // A failed "load more" leaves what is already on screen and simply stops
    // offering. The rows a lifter is reading are not worth throwing away to
    // report a page that did not arrive.
    if (!(await loadPage(items.length))) more = false;
    loadingMore = false;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-4">
  <div>
    <h2 class="text-2xl font-black text-foreground">Around the gym</h2>
    <p class="mt-1 text-sm text-muted-foreground">
      What everyone else on this install has been lifting. Open one to see the session.
    </p>
  </div>

  {#if loading}
    <!-- The same Card wrapping the same rows FeedList draws, at the same 36px
         avatar, so the first page arrives into the space already held for it. -->
    <Loading label="Loading the feed">
      <Card class="p-4">
        <SkeletonRows rows={6} />
      </Card>
    </Loading>
  {:else if failed}
    <ErrorCard message="Couldn't load the feed." onRetry={load} />
  {:else if items.length === 0}
    <!-- The single-lifter install, which is the one this app was built for. Said
         plainly and without suggesting anything has gone wrong: there is nobody
         else here, and that is a normal way to use it. -->
    <Card class="p-8 text-center">
      <p class="text-sm text-muted-foreground">
        Nothing here yet. Once someone else on this install logs a session, it'll show
        up.
      </p>
    </Card>
  {:else}
    <Card class="p-4">
      <FeedList {items} />
    </Card>

    {#if more}
      <div class="flex justify-center">
        <Button variant="outline" onclick={loadMore} disabled={loadingMore}>
          {loadingMore ? "Loading…" : "Load more"}
        </Button>
      </div>
    {/if}
  {/if}
</div>
