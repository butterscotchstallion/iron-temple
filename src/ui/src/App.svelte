<script lang="ts">
  import { onMount } from "svelte";
  import Router, { link } from "svelte-spa-router";
  import wrap from "svelte-spa-router/wrap";
  import Home from "./routes/Home.svelte";
  import Programs from "./routes/Programs.svelte";
  import ProgramDetail from "./routes/ProgramDetail.svelte";
  import SignIn from "./routes/SignIn.svelte";
  import NavBar from "./lib/NavBar.svelte";
  import HeaderBar from "./lib/HeaderBar.svelte";
  import UpdatePrompt from "./lib/UpdatePrompt.svelte";
  import ErrorBoundary from "./lib/ErrorBoundary.svelte";
  import { auth, loadMe } from "./lib/auth.svelte";
  import { loadHomeSessions } from "./lib/homeData";
  import { startPolling } from "./lib/version.svelte";

  // Hash-based routes (svelte-spa-router). "/" lands on the current program's
  // workout; "/programs" is the picker for switching.
  //
  // Everything but Home is wrapped in `asyncComponent`, which is what lets the
  // bundler split it into its own chunk. Statically importing all eleven routes
  // meant a first-time visitor downloaded the Racked report and its five
  // charts, the share-card canvas renderer and every other screen before the
  // workout in front of them could paint.
  //
  // Home stays eager because it IS the landing route — deferring it would just
  // buy a second round trip before the first paint. Programs and ProgramDetail
  // are eager for a subtler reason: Home renders both directly (it picks the
  // saved program, or the picker when there isn't one), so they land in the
  // entry chunk via that import no matter what is written here. Wrapping them
  // would only add a chunk boundary the bundler immediately inlines.
  //
  // No `loadingComponent`: on a same-origin LAN these chunks arrive in
  // milliseconds, and the codebase already prefers a beat of empty space to a
  // flash of spinner (see Home's comment on /me).
  const routes = {
    "/": Home,
    "/programs": Programs,
    "/programs/:id": ProgramDetail,
    "/sessions/:id": wrap({ asyncComponent: () => import("./routes/ActiveSession.svelte") }),
    "/library": wrap({ asyncComponent: () => import("./routes/Library.svelte") }),
    "/history": wrap({ asyncComponent: () => import("./routes/History.svelte") }),
    "/progress": wrap({ asyncComponent: () => import("./routes/Progress.svelte") }),
    // Owned by the Progress tab: it is a lift's chart, and both Progress and
    // the library link into it.
    "/exercises/:id": wrap({ asyncComponent: () => import("./routes/ExerciseProgress.svelte") }),
    // Reached from the account menu rather than the nav bar: it's a place you
    // visit now and then, not one of the four you move between while training.
    // The biggest single win from splitting — it drags in five chart components
    // and the share-card renderer.
    "/racked": wrap({ asyncComponent: () => import("./routes/Racked.svelte") }),
    "/profile": wrap({ asyncComponent: () => import("./routes/Profile.svelte") }),
    // Fallback: unknown paths go home.
    "*": Home,
  };

  // Both at once, not one after the other.
  //
  // No route can mount until /me settles, so the home screen's session list —
  // the thing the landing route is made of — could not even be REQUESTED until
  // a round trip had already completed. Starting it here overlaps the two, and
  // the cache's in-flight dedupe means Home joins this request rather than
  // making a second one.
  //
  // Signed out, this costs one 401 that nothing reads. That is the right side
  // of the trade: signing in happens once, landing signed-in happens every
  // time, and the 401 is the same request the route would have made anyway.
  onMount(() => {
    void loadMe();
    void loadHomeSessions();
  });

  // Watch for a release landing under an open tab. Owned by the shell rather
  // than by the header that displays the version, because what it feeds is the
  // update prompt — and the first poll also fills in the header's label.
  $effect(() => startPolling());
</script>

<!-- Outside <main> so the bar spans the viewport while the content below stays
     in its centred column. It carries the version, which used to sit in the
     footer. Outside the ErrorBoundary too: whatever throws below, the account
     menu stays reachable. -->
<HeaderBar />

<!-- Also outside the boundary, and outside <main>: a new build is worth
     offering whatever state the routed content got itself into — not least
     because a route that throws is one of the better reasons to take an
     update. -->
<UpdatePrompt />

<main class="mx-auto flex max-w-3xl flex-col gap-8 px-5 py-10">
  <header class="text-center">
    <a href="/" use:link class="inline-block">
      <h1
        class="font-[var(--font-display)] text-5xl font-black uppercase tracking-[0.2em] text-neon drop-shadow-[0_0_18px_rgba(176,38,255,0.7)] sm:text-6xl"
      >
        Iron Temple
      </h1>
    </a>
    {#if auth.me}
      <NavBar />
    {/if}
  </header>

  <!-- Wraps the routed content rather than the whole shell: a route that throws
       is contained, but the header and nav stay mounted so there's still a way
       out. The sign-in form is inside it too — it is the only thing a
       signed-out visitor can see, so a throw there would leave a blank page
       with no way to recover. -->
  <ErrorBoundary>
    {#if !auth.loaded}
      <!-- Render nothing rather than a spinner: /me is one local request, and a
           flash of loading state is worse than a beat of empty space. -->
    {:else if !auth.me}
      <!-- Signed out, the sign-in form replaces the router entirely, so no route
           is reachable by typing its hash. The API enforces this independently —
           this is about not showing a wall of failed requests. -->
      <SignIn />
    {:else}
      <Router {routes} />
    {/if}
  </ErrorBoundary>
</main>
