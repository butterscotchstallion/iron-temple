<script lang="ts">
  import ActivityPanel from "../lib/ActivityPanel.svelte";

  // Generated lifters and generated training, on their own screen.
  //
  // This used to be a card on /admin. It moved because the two screens answer
  // different questions — that one is "who can sign in", this one is "make the
  // install look lived in" — and because the controls had outgrown a panel: a
  // backfill, a live loop and an unattended daily schedule is three modes, each
  // with its own bounds and its own idea of what "running" means.
  //
  // Named for what it is. Everything on the wire is called *activity* and nothing
  // in the schema or any response marks a generated account — not as camouflage
  // but because nothing needs a marker: teardown re-derives the roster and checks
  // a password hash, and no lifter-facing query branches on it. See
  // docs/generated-activity.md. The personas themselves are lifting puns from the
  // same list the admin area suggests usernames from, so the feed makes no secret
  // of what they are, and an owner-only screen that called this "activity" would
  // be coy with the one person who pressed the button.
  //
  // Owner-only: the route condition in App.svelte turns the hash away for anybody
  // else, and every endpoint the panel calls sits inside the API's /admin subtree,
  // which is the check that actually counts.
</script>

<div class="flex flex-col gap-4">
  <div>
    <h2 class="text-2xl font-black text-foreground">Astroturfing</h2>
    <p class="mt-1 text-sm text-muted-foreground">
      The screens built for a shared gym — the feed, the leaderboard, another lifter's
      profile — need more than one lifter and more than one week of history before they
      show anything worth looking at. This fills that in, rather than waiting three
      months for four people to do it for real.
    </p>
  </div>

  <ActivityPanel />
</div>
