<script lang="ts">
  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import CircleQuestionMark from "@lucide/svelte/icons/circle-question-mark";

  // How a record is decided, for the two cards that list them.
  //
  // Records and milestones are the part of the app a lifter is meant to enjoy,
  // and until now nothing said how any of them were reached — so a lifter who
  // saw "est. max" against a bar that had not moved, or watched their first
  // session award nothing, had to guess. StreakCard already learned this lesson
  // for the streak; this is the same affordance for the rest of it.
  //
  // Its own component rather than inline like StreakCard's, because two surfaces
  // need it — the session recap's "Worth noting" card and the Racked report's
  // records card — and two copies of this prose is two chances for the page and
  // the page next to it to explain the same rule differently.
  //
  // The host must be `relative`: the trigger is positioned into the card's own
  // padding so it does not push the content it annotates around.
</script>

<AlertDialog.Root>
  <!-- Deliberately quiet, StreakCard's reasoning: the card is the reward, this
       is a footnote, and it only brightens when somebody goes looking for it. -->
  <AlertDialog.Trigger
    class="absolute right-2 top-2 rounded-full p-1.5 text-muted-foreground/60
           transition-colors hover:text-foreground focus-visible:outline-none
           focus-visible:ring-2 focus-visible:ring-ring"
    aria-label="How records and milestones work"
  >
    <CircleQuestionMark class="size-4" aria-hidden="true" />
  </AlertDialog.Trigger>

  <AlertDialog.Content class="sm:max-w-md">
    <AlertDialog.Header>
      <AlertDialog.Title>How records and milestones work</AlertDialog.Title>
      <AlertDialog.Description>
        A record beats your own previous best on that lift — nobody else's. There are two
        kinds: a heavier top set, marked <strong>PR</strong>, or the same bar carried for
        more reps, which raises your estimated max and is marked
        <strong>est. max</strong>. A heavier bar hides the estimated-max record it implies,
        so one achievement is told once rather than twice.
        <br /><br />
        Your first time on a lift isn't a record. There was nothing to beat yet, so it's
        listed as a first time instead — a lift you've just taken up shouldn't arrive
        looking like a personal best.
        <br /><br />
        Milestones are the weights lifters say out loud — 95, 135, 185, 225 and up — plus
        lifetime tonnage marks. Each one is yours once per lift, ever, which is what makes
        them worth having.
        <br /><br />
        None of this counts a set you didn't log. A bar you loaded and walked away from
        earns nothing.
      </AlertDialog.Description>
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>Got it</AlertDialog.Cancel>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
