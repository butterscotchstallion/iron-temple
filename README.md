# iron-temple

A fitness tracker for barbell training. You pick a program, it tells you what to
lift and how much, you tap through the sets, and it works out the next session's
weights from what you actually did.

It is built for one lifter on a phone at the rack — registration closes after the
first account, there is no feed and no social layer, and every screen is sized for
a thumb between sets. Go + chi API (`src/api`), Svelte + Tailwind UI (`src/ui`),
PostgreSQL.

## Features

### Programs

Six programs ship seeded, all from the StrongLifts family plus Madcow: **StrongLifts
5x5** and its **Lite**, **Mini** and **Intermediate** variants, **Advanced 3x5** for
when 5x5 stalls, and **Madcow 5x5**. Each is a set of days (Workout A, B, C) holding
the prescribed lifts, sets and reps.

- **Put the days on a calendar.** Assign a weekday to each program day, and the app
  knows what you're scheduled to do and when you missed it.
- **Add your own assistance work.** Accessories attach to a program day as a
  per-account overlay: the seeded programs stay untouched, so your curls never move
  anyone else's Workout A. Give one a **rep range** and it runs on double
  progression — add reps inside the range, and when every set reaches the top the
  weight goes up and the reps reset to the bottom. Without a range it simply
  carries forward whatever you last lifted.
- **See the next session before you start it.** Every day previews its target
  weights, so you know what's coming without opening a session.

### Sessions

- **Set-by-set logging** — tick sets off as you complete them, or edit reps and
  weight when the day doesn't go to plan. **Add a set** for the extra one you had
  in you, or **drop one** you skipped.
- **A rest timer that knows the lift.** Rest is a property of the movement, not a
  flat three minutes: a deadlift and a lateral raise get their own lengths, and
  assistance work inherits one automatically.
- **Plate math**, drawn as a loaded bar, so you don't do the arithmetic in your
  head — and drawn with **your** bar and **your** plates, so it never calls for a
  fourth pair of 45s you don't own. A weight the rack can't build rounds down to
  one it can, and says so.
- **Bodyweight weigh-ins** recorded against the session.
- **Finish the session** to close it out — with confetti when it went well.

### Progression

The linear engine sets the next weight for every lift from its own history:
**+5 lb** after a successful session (**+10 lb** on the deadlift), the same weight
again after a failure, and a **deload to 90%** after three consecutive failures at
one weight. It also says *why* it picked a number, so a deload or an approaching
stall reads as a decision rather than a mystery.

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
- Bodyweight trend, when there are weigh-ins to draw.
- An **archetype** naming how you trained, and a **share card** you can export.

Monthly and yearly recaps also go out **by email** when a mail relay is configured.
The reporter asks the database which completed periods haven't been sent rather than
waking on the 1st, so downtime delays a recap instead of dropping it.

### Library, history and progress

- **Exercise library** — everything seeded plus your own movements, each with a
  muscle group and its equipment, grouped so a long list stays navigable.
- **Your gym** — the bar's weight and the plates you own, on the profile. Every
  weight the app draws is loaded onto them.
- **Per-lift history** with a progress chart and your top set to date.
- **Session history**, paged, with lifetime volume across everything you've logged.
- **Home** opens on the program you last used, with your streak and a heatmap of
  recent training.

### Accounts

Password login on a session cookie, with **registration closing as soon as the first
account exists** — this is a homelab app that expects exactly one lifter, and an open
signup form on the public internet is an open door. Profile carries a display name,
an uploaded avatar or a colour, and a password change.

## Documentation

| | |
|--|--|
| [`docs/development.md`](docs/development.md) | Git hooks, preflight gates, integration tests |
| [`docs/design.md`](docs/design.md) | Original design brief |
| [`docs/implementation-plan.md`](docs/implementation-plan.md) | What's built, phase by phase |
| [`docs/api-integration-tests.md`](docs/api-integration-tests.md) | The DB-backed API suite |
| [`docs/ci-branch-protection.md`](docs/ci-branch-protection.md) | CI and branch rules |
| [`docs/sandbox-ui-tooling.md`](docs/sandbox-ui-tooling.md) | Offline UI tooling in the sandbox |
| [`AGENTS.md`](AGENTS.md) | Commit conventions and generated-code rules |

`src/api/openapi.yaml` is the API contract; the UI's typed client is generated from
it. Per-project setup lives in `src/api` (see its `Makefile`) and `src/ui` (see its
`README.md`).
