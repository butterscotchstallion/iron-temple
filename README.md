# iron-temple

A fitness tracker for barbell training. You pick a program, it tells you what to
lift and how much, you tap through the sets, and it works out the next session's
weights from what you actually did.

It is built for a phone at the rack — self-registration closes after the first
account, and every screen is sized for a thumb between sets. One lifter is the
expected install and the one everything is tuned for; where a household or a gym
crew share a box, the owner adds the other accounts by hand and they can see each
other's training. Go + chi API (`src/api`), Svelte + Tailwind UI (`src/ui`),
PostgreSQL.

## Features

### Programs

Eight programs ship seeded, and you can build your own. The eight are from the
StrongLifts family plus Madcow:
**StrongLifts 5x5** and its **Lite**, **Mini** and **Intermediate** variants,
**Advanced 3x5** for when 5x5 stalls, and **Madcow 5x5**. **Lite (Dumbbell Press)**
is Lite with the overhead press taken to dumbbells, for a rack without the ceiling
height or the shoulders for a barbell. **Glutes & Legs (Bands and Free Weights)** is
the odd one out — two days, squat-led and hinge-led, mixing barbell and dumbbell work
with resistance bands. Each is a set of days (Workout A, B, C) holding the prescribed
lifts, sets and reps.

- **Band work is prescribed by band, not by weight.** A band carries no poundage, so
  there is no honest number to put on it: banded movements log at 0 lb and stay
  there, and the program says which band to pull for each one. The same rule that
  keeps bodyweight work at bodyweight keeps them there — a lift worked at 0 has
  nothing to add to, so a good session never turns a banded lateral walk into a 5 lb
  banded lateral walk. Decide to hang a plate off one and it progresses like anything
  else from then on; entering a load is what opts a lift in.

- **Build your own.** Start from nothing, or take a copy of one that already works
  and change what you need — swap the overhead press for dumbbells, drop a lift your
  shoulder doesn't like. Name the days, put the lifts on them in the order you do
  them, and set where each one starts. **A copy leaves the original alone**: the
  eight seeded programs belong to the install and stay exactly as they are for
  everybody, which is a property of the schema rather than a rule somebody has to
  remember. **Your lifts keep their history**, because history follows the lift
  and not the program — a squat in a program you built this morning picks up the
  weight you were already squatting rather than sending you back to an empty bar.

  You choose whether the rest of the install can see it. **Sharing controls who can
  find it, not who can keep it**: turn sharing off and it leaves everyone else's
  picker, but a lifter already running it keeps it, because their sessions are tied
  to it either way and taking it away would cost them their training to no purpose.
  Only you can change it.

  Finished with one? **Archive it.** It leaves the picker and stays trainable, and
  every session you logged against it stays exactly where it is — a program you have
  trained can't be deleted, because deleting it would mean deleting the work. The same
  rule applies one level down: take a day out of a program and it goes, but if you
  have trained it the workout stays in your history and keeps its name.
- **Put the days on a calendar.** Assign a weekday to each program day, and the app
  knows what you're scheduled to do and when you missed it.
- **Add your own assistance work, and have it progress like everything else.**
  Accessories attach to a program day as a per-account overlay: the seeded programs
  stay untouched, so your curls never move anyone else's Workout A. They run the
  **same linear engine** as the prescribed lifts — hit your reps on every set and
  the weight goes up next time, miss and it repeats, miss three times and it
  deloads. The jump is the smallest one your equipment admits, never a programme
  pace. Bodyweight work stays bodyweight: a lift logged at 0 has nothing to add to.
  Prefer something gentler on light isolation work? Give a lift a **rep range** on
  the program page and it switches to double progression — climb reps inside the
  range, and only when every set reaches the top does the weight move, with no
  deload ever.
- **See the next session before you start it.** Every day previews its target
  weights, so you know what's coming without opening a session.
- **Always know what's next.** The program page is a queue: every day carries the
  date it next comes round, soonest first, so the top two cards are the next two
  workouts. Finish today's and it doesn't vanish — it moves to the back wearing
  next week's date, which on a two-day program leaves you looking at the rest of
  this week and the start of the next. A day you trained today is marked done,
  and offers to show you that session rather than start another; one you walked
  away from mid-workout stays due today and offers to pick it back up.

### Sessions

- **Set-by-set logging** — tick sets off as you complete them, or edit reps and
  weight when the day doesn't go to plan. **Add a set** for the extra one you had
  in you, or **drop one** you skipped.
- **Add a lift you decided on at the rack.** Assistance can be added from the
  workout you're in, not just from the program page — it drops into today's
  session straight away *and* joins the day it came from, rep range and all, so it
  comes round again with a progression behind it rather than being a one-off. The
  weight starts where you left it, and like everything else here it works with no
  signal.
- **Training without a signal.** A gym in a basement is where this app is used and
  where the network isn't. A tap that can't reach the server is written to disk,
  applied to the screen as though it had landed, and replayed in order when the
  API answers again — so a dropped connection costs a banner rather than a set.
  It survives the tab being killed and the phone being pocketed; the sets are
  sent when you walk out. (A **reload** while offline is the one gap: the queued
  work is safe and will sync, but the screen can't be rebuilt without the server,
  so it waits.)
- **A rest timer that knows the lift.** Rest is a property of the movement, not
  one number for everything: a squat and a lateral raise get their own lengths,
  and assistance work inherits one automatically. Three minutes is the ceiling —
  nothing rests longer than that.
- **Plate math**, drawn as a loaded bar, so you don't do the arithmetic in your
  head — and drawn with **your** bar and **your** plates, so it never calls for a
  fourth pair of 45s you don't own. A weight the rack can't build rounds down to
  one it can, and says so.
- **Bodyweight weigh-ins** recorded against the session.
- **Finish the session** to close it out — with confetti when it went well — and
  land on a **recap** of what it was worth: how long it took and where that
  ranks among your other goes at the same workout, the tonnage restated as
  something you can picture, the **percentage the weights moved** since that day
  last came round, any **records and milestones** you hit, your streak, which
  **muscle groups** the work went to, and every lift with its own
  before-and-after. It ends with **what you earned** — the weights your next
  session of that day will prescribe, and the engine's reasoning, so a deload
  reads as a decision rather than a surprise a week later.

  It's a page of its own, reachable again from **history** or from any finished
  session, and there's a **share card** to export. The statistics are the ones
  Racked computes over a month, pointed at a single session — a record announced
  at the rack and the same record listed in March are decided by the same code.
  It's also the one screen built to survive a dead network: what can be worked
  out from the workout you just did is drawn from memory, the rest is marked as
  waiting, and it fills itself in the moment you're back on signal.

### Progression

The linear engine sets the next weight for every lift from its own history —
prescribed and assistance alike: **+5 lb** after a successful session (**+10 lb**
on the deadlift), the same weight again after a failure, and a **deload to 90%**
after three consecutive failures at one weight. A deload always lands *below* the
weight that stalled: where 10% is finer than the grid, it takes one step off
instead, because a "deload" that rounds back to the weight you just failed three
times is no deload at all. It also says *why* it picked a number, so a deload or
an approaching stall reads as a decision rather than a mystery.

Every jump is one **your** equipment can make. The smallest change a barbell admits
is twice your lightest plate, and the smallest a pair of dumbbells admits is twice
your rack's step — a weight here is the whole load, so a rack of 5 lb bells moves
the pair 10 at a time and has nothing in between. Both come off your gym setup
rather than a constant, so owning 1.25s means a deload can land on 121.5, and a
rack of 2.5s means a dumbbell press climbs 5 lb a session instead of 10.

The programme's pace is separate from that grid and is rounded **up** to it: the
squat still wants +5 and the deadlift +10, and a gym too coarse to build them gets
the next weight that exists rather than an advance of nothing. The grid alone
sets the increase on assistance work, on either rule — a curl is not a squat, so
it goes up by the least the rack allows rather than at a programme's pace.

History follows the **lift**, not the program — so taking Advanced 3x5 when your
5x5 stalls picks the bar up where you left it instead of sending you back to an
empty one.

**Madcow** runs differently, because Madcow is different. Each day ramps to a top
set — 50, 62.5, 75, 87.5, 100% — and the top set moves **once a week**, on the one
day that reaches it; every other day's weights are a percentage of that number. The
light day stops a rung short, and the intensity day finishes above it with a triple
and then a backoff.

Where a lift **starts** is yours to set, since the seeded starting weights assume a
45 lb bar and yours might not be one.

### Racked — your training, in review

A recap of **this week, this month or this year**, computed from performed sets:

- Volume, sessions, sets and reps, with the change against the previous period.
- Volume split by **muscle group** and by **prescribed vs. assistance** work.
- **PRs, milestones, heaviest set, fastest session** and your **most improved** lift.
- **Streaks and attendance** measured against the days you scheduled — and against
  the days that have actually elapsed, so a month two days in isn't graded as a
  whole one.
- Per-lift trend charts, a calendar heatmap, best weekday and peak training hour.
  The heatmap draws a row per weekday that means something — the ones your program
  is scheduled on, plus any you actually trained — rather than a fixed seven. On a
  two-day program that is two rows instead of five-sevenths empty space, and a blank
  cell on a scheduled row is a session missed rather than a Tuesday.
- Bodyweight trend, when there are weigh-ins to draw.
- An **archetype** naming how you trained, and a **share card** you can export.

Monthly and yearly recaps also go out **by email** when a mail relay is configured.
The reporter asks the database which completed periods haven't been sent rather than
waking on the 1st, so downtime delays a recap instead of dropping it.

### Library, history and progress

- **Exercise library** — everything seeded plus your own movements, each with a
  muscle group and its equipment, grouped so a long list stays navigable. Open
  any seeded movement and a little figure shows you how it goes.

  The figures are **drawn, not filmed**: a movement is stored as a handful of
  joint angles and the shape is computed in the browser, the way the plate math
  draws your bar rather than photographing it. That is what keeps them working
  in a basement — there is no clip to fetch, so there is nothing to fail when
  the signal doesn't. They follow the theme, and a lifter who has asked their
  phone for less movement gets the single most informative pose instead of a
  loop.

  Not every movement has one. A Russian twist is a rotation that a side view
  cannot show and a front view draws as sitting still, and a shrug moves about
  two inches — drawing those would teach something untrue, so they keep their
  icon and say nothing. Your own movements have no figure either, for the
  ordinary reason that nobody has drawn them.
- **Your gym** — the bar's weight, the plates you own, and what your dumbbell rack
  steps by, on the profile. Every weight the app draws is loaded onto them, and
  every jump it prescribes is one they can build.
- **Per-lift history** with a progress chart and your top set to date.
- **Session history**, paged, with lifetime volume across everything you've logged.
- **Home** opens on the program you last used, with your streak and a heatmap of
  recent training — collapsed to the weekdays your program runs on, so the days you
  were meant to train and didn't are visible rather than lost among the rest days.

### Accounts

Password login on a session cookie, with **registration closing as soon as the first
account exists** — this is a homelab app that expects one lifter, and an open
signup form on the public internet is an open door. Profile carries a display name,
an uploaded avatar or a colour, and a password change.

The owner can add further accounts by hand, each starting with a one-time password
it must replace before it can do anything else. Where an install has more than one,
**Lifters** lists them, and opening one shows what they have been lifting: their
lifetime tonnage, the month's volume, streak and muscle split, and a heatmap of the
days they trained. **Around the gym** is the same information the other way up — a
feed of everyone else's recent sessions, each opening onto the full recap of that
workout. It shows up as a card at the foot of home once there is anything in it, and
it holds *other* people's sessions only: your own already have a history page, so a
feed of everybody would be that page again under another name. Which is also why a
one-lifter install never sees the card at all — there is nothing for it to hold, so
home looks exactly as it always did.

A **leaderboard** compares everyone over a week, month or year. It opens on how
often each lifter trained and how well they kept their *own* schedule, because those
are measures relative to the lifter — raw tonnage ranks people by bodyweight and
training age as much as by effort, so it's the last board rather than the first, and
the page says so where it's drawn. Every figure is read off the same Racked report
that lifter sees on their own page; nothing is recomputed. A lifter appears on a
board when the metric *means* something for them, which isn't the same as non-zero:
somebody who trained nothing is listed at zero, but somebody whose program has no
scheduled weekdays has no attendance to grade and is left off that board entirely.

Every session carries **applause and conversation**: four reactions, and comments
capped at a couple of sentences. You can't applaud your own workout — that's what the
reaction is for — but you can comment on it, because answering somebody is the obvious
thing to want. The author can delete their own comment and the install's owner can
delete any, which is the only moderation a household needs. None of it goes through
the offline queue: a rep tapped at the rack has to survive a dead network, and a
reaction given from the sofa does not. Every figure there is the *same* report the lifter reads on their
own Racked page, computed by the same code over the same rows — so the install can
never hold two answers about one history. There is no visibility setting: accounts
exist only because the owner created them, so admission is the consent. The gym
setup is the one thing that stays private, because another lifter's bar and plates
decide their weights and must never be read as a basis for yours.

## Documentation

| | |
|--|--|
| [`docs/development.md`](docs/development.md) | Git hooks, preflight gates, sqlc, integration tests |
| [`docs/design.md`](docs/design.md) | Original design brief |
| [`docs/implementation-plan.md`](docs/implementation-plan.md) | What's built, phase by phase |
| [`docs/api-integration-tests.md`](docs/api-integration-tests.md) | The DB-backed API suite |
| [`docs/ci-branch-protection.md`](docs/ci-branch-protection.md) | CI and branch rules |
| [`docs/observability.md`](docs/observability.md) | Prometheus metrics and how to scrape them |
| [`docs/generated-activity.md`](docs/generated-activity.md) | Filling an install with lifters and history to look at |
| [`docs/sandbox-ui-tooling.md`](docs/sandbox-ui-tooling.md) | Offline UI tooling in the sandbox |
| [`AGENTS.md`](AGENTS.md) | Commit conventions and generated-code rules |

`src/api/openapi.yaml` is the API contract; the UI's typed client is generated from
it. Per-project setup lives in `src/api` (see its `Makefile`) and `src/ui` (see its
`README.md`).
