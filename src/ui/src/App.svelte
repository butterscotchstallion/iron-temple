<script lang="ts">
  import Router, { link } from "svelte-spa-router";
  import Home from "./routes/Home.svelte";
  import Programs from "./routes/Programs.svelte";
  import ProgramDetail from "./routes/ProgramDetail.svelte";
  import ActiveSession from "./routes/ActiveSession.svelte";
  import History from "./routes/History.svelte";
  import Progress from "./routes/Progress.svelte";
  import ExerciseProgress from "./routes/ExerciseProgress.svelte";
  import NavBar from "./lib/NavBar.svelte";
  import Footer from "./lib/Footer.svelte";
  import ErrorBoundary from "./lib/ErrorBoundary.svelte";

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
    // Fallback: unknown paths go home.
    "*": Home,
  };
</script>

<main class="mx-auto flex max-w-3xl flex-col gap-8 px-5 py-10">
  <header class="text-center">
    <a href="/" use:link class="inline-block">
      <h1
        class="font-[var(--font-display)] text-5xl font-black uppercase tracking-[0.2em] text-neon drop-shadow-[0_0_18px_rgba(176,38,255,0.7)] sm:text-6xl"
      >
        Iron Temple
      </h1>
    </a>
    <NavBar />
  </header>

  <!-- Wraps the router rather than the whole shell: a route that throws is
       contained, but the header and nav stay mounted so there's still a way out. -->
  <ErrorBoundary>
    <Router {routes} />
  </ErrorBoundary>

  <Footer />
</main>
