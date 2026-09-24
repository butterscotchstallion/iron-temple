<script lang="ts">
  import { onMount } from "svelte";
  import Router, { link } from "svelte-spa-router";
  import wrap from "svelte-spa-router/wrap";
  import Home from "./routes/Home.svelte";
  import Programs from "./routes/Programs.svelte";
  import ProgramDetail from "./routes/ProgramDetail.svelte";
  import SignIn from "./routes/SignIn.svelte";
  import ForcePasswordChange from "./routes/ForcePasswordChange.svelte";
  import NavBar from "./lib/NavBar.svelte";
  import ErrorBoundary from "./lib/ErrorBoundary.svelte";
  import OfflineBanner from "./lib/OfflineBanner.svelte";
  import Toaster from "./lib/Toaster.svelte";
  import { auth, loadMe } from "./lib/auth.svelte";
  import { loadHomeSessions } from "./lib/homeData";
  import { startPolling as startNotificationPolling } from "./lib/notifications.svelte";
  import { startAchievementPolling } from "./lib/achievements.svelte";
  import { startLive } from "./lib/live.svelte";
  import { startPolling } from "./lib/version.svelte";
  import { deferred } from "./lib/deferred.svelte";
  import { watchConnectivity } from "./lib/connectivity.svelte";
  import { flush, startQueueRetry } from "./lib/writeQueue.svelte";

  // The two pieces of shell built on bits-ui, fetched rather than bundled — it
  // and its dependencies were about a third of the entry chunk, to render a
  // dropdown, a popover and a dialog that a first paint has no use for. See
  // deferred.svelte.ts.
  //
  // Deferred here, at the one place that mounts them, rather than by splitting
  // each component around its own trigger: this needs no stand-in markup to
  // keep in step with the real thing, and leaves their tests untouched.
  //
  // Nothing visible waits on it. The header's contents are the signed-in user
  // and the running version — both of which arrive from /me and /health, so on
  // a cold load there is nothing to draw there until the network answers
  // regardless of when this chunk lands.
  const headerBar = deferred(() => import("./lib/HeaderBar.svelte"));
  const updatePrompt = deferred(() => import("./lib/UpdatePrompt.svelte"));

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
    // Declared BEFORE "/programs/:id", because svelte-spa-router matches in
    // insertion order. Below it, "new" would match as an id, ProgramDetail would
    // compute Number("new") = NaN, and the screen would fetch /programs/NaN.
    "/programs/new": wrap({
      asyncComponent: () => import("./routes/ProgramCreate.svelte"),
    }),
    "/programs/:id/edit": wrap({
      asyncComponent: () => import("./routes/ProgramEdit.svelte"),
    }),
    "/programs/:id": ProgramDetail,
    "/sessions/:id": wrap({ asyncComponent: () => import("./routes/ActiveSession.svelte") }),
    // Where finishing a workout lands, and reachable afterwards by its own URL.
    // Split for the reason /racked is: it drags in the share-card renderer.
    "/sessions/:id/recap": wrap({
      asyncComponent: () => import("./routes/SessionRecap.svelte"),
    }),
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
    // The profile is a shell with five sections behind a sub-nav, one per URL,
    // so both paths land on the same component: the bare one redirects to the
    // default section rather than rendering a sixth thing. Two entries pointing
    // at one dynamic import costs no second chunk — the bundler emits it once.
    "/profile": wrap({ asyncComponent: () => import("./routes/Profile.svelte") }),
    "/profile/:section": wrap({
      asyncComponent: () => import("./routes/Profile.svelte"),
    }),
    // Account management, for the one account that claimed the install.
    //
    // The condition is the router's own guard: a failing one falls through to
    // the next matching route, which is the "*" below, so a non-admin typing
    // #/admin lands on Home rather than on a screen of 403s. It is not the
    // security boundary — the API refuses /admin/* with admin_required whatever
    // the client believes, and that is the check that counts.
    "/admin": wrap({
      asyncComponent: () => import("./routes/Admin.svelte"),
      conditions: [() => auth.me?.isAdmin === true],
    }),
    // Generated activity, and the only screen in the app whose subject is that
    // some of the lifters aren't real. Its own area rather than a panel on
    // /admin: that screen is about who can sign in, this one is about populating
    // the install, and the two were only ever adjacent because both are
    // owner-only.
    //
    // Same route condition as /admin, and the same caveat — it turns the hash
    // away for tidiness, while the API refuses /admin/activity/* with
    // admin_required whatever the client believes. Note the endpoints stay under
    // /admin and are still named for activity rather than for this screen: the
    // wire deliberately gives nothing away about which accounts are generated.
    "/astroturfing": wrap({
      asyncComponent: () => import("./routes/Astroturfing.svelte"),
      conditions: [() => auth.me?.isAdmin === true],
    }),
    // Other lifters on this install. Reached from the account menu, not the nav
    // bar — see Lifters.svelte — and split for the same reason /racked is: the
    // profile drags in the heatmap and two bar charts.
    //
    // No route condition, unlike /admin above: every account may read these, so
    // there is nothing for the client to guard. The API is the boundary either
    // way.
    "/lifters": wrap({ asyncComponent: () => import("./routes/Lifters.svelte") }),
    "/lifters/:id": wrap({ asyncComponent: () => import("./routes/LifterProfile.svelte") }),
    // Reached from the card at the foot of Home, and from the account menu. Note
    // the card is NOT this route in miniature — it loads four rows and renders
    // nothing at all when there are none, which is what keeps a single-lifter
    // install looking untouched. See FeedCard.svelte.
    "/feed": wrap({ asyncComponent: () => import("./routes/Feed.svelte") }),
    "/leaderboard": wrap({ asyncComponent: () => import("./routes/Leaderboard.svelte") }),
    // Another lifter's session recap. Its own route rather than a mode of
    // /sessions/:id/recap: that one is scoped to the caller and would 404 here,
    // and the two read different endpoints.
    "/lifters/:lifterId/sessions/:sessionId/recap": wrap({
      asyncComponent: () => import("./routes/LifterSessionRecap.svelte"),
    }),
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
    // Alongside the data, not after it: these are separate connections, and the
    // shell should be in place by the time there is anything to put in it.
    void headerBar.load();
    void updatePrompt.load();
    // Anything left in the write queue from a previous visit — a session logged
    // in a basement and then backgrounded, a tab the phone killed on the drive
    // home. Replayed at startup rather than when the session screen next mounts,
    // because the lifter has no reason to ever open that screen again.
    void flush();
  });

  // Owned by the shell for the same reason the version poll is: the queue
  // outlives any one route, and the reps waiting in it belong to a session
  // screen that may never be mounted again.
  $effect(() => watchConnectivity(() => void flush()));
  $effect(() => startQueueRetry());

  // Watch for a release landing under an open tab. Owned by the shell rather
  // than by the header that displays the version, because what it feeds is the
  // update prompt — and the first poll also fills in the header's label.
  $effect(() => startPolling());

  // Watch for applause and conversation arriving. Owned by the shell rather
  // than by the bell that draws it, because HeaderBar is lazy-loaded: the bell
  // does not exist for the first moments of a page load, and whether anybody is
  // counting must not depend on which chunk has arrived yet.
  //
  // Reading auth.me inside the effect is what makes this restart on sign-in and
  // tear down on sign-out — the teardown it returns runs before each re-run.
  // The password-change gate matches HeaderBar's: an account in that state is
  // refused by every endpoint except the two it needs to escape, so polling
  // would be a 403 a minute for a panel it cannot act on.
  $effect(() => {
    if (!auth.me || auth.me.mustChangePassword) return;
    return startNotificationPolling();
  });

  // Who is wearing which crown. Owned here on the same terms and for the same
  // reason as the poll above — the surfaces that draw a crown are lazy-loaded,
  // so whether anybody is keeping the standings current must not depend on
  // which chunk has arrived. It asks far less often; see the module.
  $effect(() => {
    if (!auth.me || auth.me.mustChangePassword) return;
    return startAchievementPolling();
  });

  // The live socket, on exactly the same terms and for the same reasons: the
  // /live route is behind both gates too, so an account that cannot poll
  // cannot connect either. The poller above does not stop when this connects —
  // it backs off to a safety net, because a proxy that eats upgrades has to
  // leave a working app behind. See live.svelte.ts.
  $effect(() => {
    if (!auth.me || auth.me.mustChangePassword) return;
    return startLive();
  });
</script>

<!-- Outside <main> so the bar spans the viewport while the content below stays
     in its centred column. It carries the version, which used to sit in the
     footer. Outside the ErrorBoundary too: whatever throws below, the account
     menu stays reachable. -->
{#if headerBar.current}
  <headerBar.current />
{:else}
  <!-- The bar's own shell, holding its exact height while the chunk is on its
       way, so the page below never jumps. Empty rather than skeletonised: what
       goes in here is a name and a version string that the network has not
       returned yet either, and a shimmer standing in for two short labels is
       more distracting than the space they will occupy. -->
  <header
    class="sticky top-0 z-40 w-full border-b border-white/10 bg-black/95"
    aria-hidden="true"
  >
    <div class="h-12"></div>
  </header>
{/if}

<!-- Also outside the boundary, and outside <main>: a new build is worth
     offering whatever state the routed content got itself into — not least
     because a route that throws is one of the better reasons to take an
     update. Nothing stands in for it while it loads: it renders nothing at all
     until a release lands. -->
{#if updatePrompt.current}
  <updatePrompt.current />
{/if}

<!-- Above the routed content and outside the boundary, like the update prompt.
     What it reports is true of the whole app rather than of one screen, and it
     has to survive navigating away from the session that filled the queue. -->
<OfflineBanner />

<!-- Fixed to the viewport, so its position in the markup decides nothing about
     where it draws. It is here for the reason the two banners above are: what it
     reports has to outlive the route that raised it, and it must not be inside
     the boundary that catches a route throwing. -->
<Toaster />

<main class="mx-auto flex max-w-3xl flex-col gap-8 px-5 py-10">
  <header class="text-center">
    <a href="/" use:link class="inline-block">
      <h1
        class="font-[var(--font-display)] text-5xl font-black uppercase tracking-[0.2em] text-neon drop-shadow-[0_0_18px_rgba(176,38,255,0.7)] sm:text-6xl"
      >
        Iron Temple
      </h1>
    </a>
    {#if auth.me && !auth.me.mustChangePassword}
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
      <!-- Nothing to LOOK at, still. /me is one local request, and a flash of
           loading state is worse than a beat of empty space — but the deeper
           reason no skeleton goes here is that there is no one shape to hold:
           what follows is either the sign-in form or a whole routed app, and a
           placeholder would have to pick one and be wrong the other half of the
           time. Every route behind this draws its own, shaped to itself.

           What was missing was the other half of an indicator. The bars
           elsewhere are aria-hidden, so a screen reader arriving here found an
           empty document and nothing to wait for; this is the announcement
           without the shimmer. -->
      <p class="sr-only" role="status" aria-busy="true">Signing you in…</p>
    {:else if !auth.me}
      <!-- Signed out, the sign-in form replaces the router entirely, so no route
           is reachable by typing its hash. The API enforces this independently —
           this is about not showing a wall of failed requests. -->
      <SignIn />
    {:else if auth.me.mustChangePassword}
      <!-- An account the admin created, still holding the password they were
           given. Replaces the router for the same reason SignIn does: the API
           answers 403 password_change_required on everything else, so there is
           no route behind this worth reaching, and leaving one mounted would
           only paint a screen of failures.

           Truthy rather than === true on purpose. The field is required by the
           spec, but the e2e specs stub /me with hand-written objects that
           predate it, and "absent" has to mean the same as false — which is
           also what the API's own docs promise. -->
      <ForcePasswordChange />
    {:else}
      <Router {routes} />
    {/if}
  </ErrorBoundary>
</main>
