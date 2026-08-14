<script lang="ts">
  import { onMount } from "svelte";
  import Router, { link } from "svelte-spa-router";
  import Home from "./routes/Home.svelte";
  import Programs from "./routes/Programs.svelte";
  import ProgramDetail from "./routes/ProgramDetail.svelte";
  import ActiveSession from "./routes/ActiveSession.svelte";
  import History from "./routes/History.svelte";
  import Progress from "./routes/Progress.svelte";
  import ExerciseProgress from "./routes/ExerciseProgress.svelte";
  import Profile from "./routes/Profile.svelte";
  import SignIn from "./routes/SignIn.svelte";
  import NavBar from "./lib/NavBar.svelte";
  import HeaderBar from "./lib/HeaderBar.svelte";
  import ErrorBoundary from "./lib/ErrorBoundary.svelte";
  import { auth, loadMe } from "./lib/auth.svelte";

  // Hash-based routes (svelte-spa-router). "/" lands on the current program's
  // workout; "/programs" is the picker for switching.
  const routes = {
    "/": Home,
    "/programs": Programs,
    "/programs/:id": ProgramDetail,
    "/sessions/:id": ActiveSession,
    "/history": History,
    "/progress": Progress,
    "/exercises/:id": ExerciseProgress,
    "/profile": Profile,
    // Fallback: unknown paths go home.
    "*": Home,
  };

  onMount(loadMe);
</script>

<!-- Outside <main> so the bar spans the viewport while the content below stays
     in its centred column. It carries the version, which used to sit in the
     footer. Outside the ErrorBoundary too: whatever throws below, the account
     menu stays reachable. -->
<HeaderBar />

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
