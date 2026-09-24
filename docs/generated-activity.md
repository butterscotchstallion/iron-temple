# Generated activity

The screens built for a shared gym — the feed, the leaderboard, another lifter's
profile — need more than one lifter and more than one week of history before they
show anything worth looking at. This fills that in, so you can see them without
waiting three months for four people to do it for real.

Find it on **Account menu → Astroturfing**. It is owner-only.

The screen is named for what it does, which the rest of the system deliberately is
not: nothing on the wire, in the schema or in any response calls these accounts
generated. They are ordinary accounts that happen to have been made by a loop, and
the endpoints under this screen stay named for *activity* — the one screen only its
owner can see is where the quiet part gets said out loud.

The **names** are not part of that reticence. The personas are lifting puns on
famous names — Judi Bench, Dua Lats, Clint Beastwood — the same joke the admin
area's "add someone" form suggests usernames from. Anybody who reads the feed will
work out that Keanu Heaves does not train here, and that is the intent: an install
being filled in for the look of it should be obvious to the people looking at it.
What the system withholds is a *marker*, and only because nothing needs one —
clean-up re-derives the roster and checks a password hash, and no lifter-facing
query has any business branching on whether an account was typed in by hand.

## What it does

**Generate history** creates lifters and walks forward through the weeks you ask
for, logging sessions on each one's scheduled days, then a pass of reactions and
comments. A few lifters over a few months takes a couple of seconds.

**Live** starts a loop where one lifter acts per tick — a session, a reaction or a
comment — so things arrive while you have a screen open. Useful for watching the
feed move; pointless for anything historical, which is what the backfill is for. It
dies with the process.

**Daily** is the unattended one, and the only one that survives a restart. Switch it
on and the install keeps generating a day's training each day, on the lifters' own
scheduled weekdays, without anybody pressing anything.

**Clean up** removes the generated lifters and everything of theirs.

## What it tells you while it runs

Anything that worked raises a **toast** — bottom of the screen, gone in six
seconds. Anything that failed does not: it goes to the error banner on the panel,
which stays until dismissed. The split is the point. "Generated 137 sessions" is a
moment worth reading once; "check the server log" is an errand, and it must still
be there when you come back from the log.

Two of those notices are worth calling out.

The **backfill's** notice opens before the request and resolves in place into the
counts, because that call is synchronous and three months across four lifters is
several seconds of a screen with nothing arriving on it. It has no dismiss button
while it is pending — there is nothing to dismiss yet, and taking it down would
leave a screen that looks idle with the request still open.

The **live loop** raises one per tick, in the server's own words — "Judi Bench
logged Workout A" — which is the actual reason to leave this screen open. Work done
*before* the screen opened is deliberately silent: the panel compares each poll
against the previous one rather than against a stored count, so a loop that has
been ticking for an hour does not greet you with forty toasts. If a tick produced
more than one action the notice says how many it could not name, because only the
last one carries a description.

**The daily run cannot toast**, and that is not an oversight worth fixing. It runs
whether or not anybody has a browser open — that is the whole point of it — and the
panel only polls while the live loop is going. What it last did is reported on the
panel instead, which is the honest channel for something that happened while you
were asleep.

## How the daily run works

It is **a ticker that asks, not an alarm that fires**. Every hour it asks the
database which of the last few days have no run recorded, and generates those. It
does not wake at midnight.

That distinction is the whole design, and it is borrowed from the Racked reporter
(`report_runs`, migration 0008) which solved the same problem first: a clock-driven
job that misses its instant has missed it, where a question asked hourly is answered
correctly the moment the process comes back. So a restart, a closed laptop or a
deploy **delays** a day's activity instead of losing it.

Three consequences worth knowing:

- **Switching it on takes effect within the hour**, not immediately. That is the
  cost of having no boot-time flag and needing no restart. Press *Generate history*
  if you don't want to wait.
- **A day is generated at most once, including on the failure path.** The claim,
  every write and the recorded counts all happen in **one transaction**, so a
  rollback takes the claim with it and an aborted day is indistinguishable from one
  nobody touched. There is no release step — a rollback *is* the release. Repeated
  ticks, a restart, or more than one replica all end up safe because correctness comes
  from the primary key rather than from counting processes.
- **Recognition is bounded twice.** A day's pass only looks at sessions from the last
  couple of days, and a lifter never comments twice on the same session. Both matter
  because a catch-up runs one pass per day it covers: unbounded, eight days of
  catch-up stacked eight rounds of comments onto the oldest sessions in a single tick.
  Reactions are already idempotent on their primary key; comments are not, and must
  not be, because a real lifter replying twice is legitimate — so the guard lives in
  the generator.
- **Catch-up is bounded to a week.** Long enough for a weekend or a deploy; short
  enough that a month away does not produce a month of training in a single tick.

The schedule lives in `generated_activity_schedule` — one row, enforced by a
`CHECK (id = 1)` so "one row" is a property of the schema rather than a convention.
It is **off** after the migration, so an existing install generates nothing until its
owner asks.

`DueDays` is a pure function of the clock, exactly as `racked.DuePeriods` is, which
is what makes month ends, year ends and leap days testable without a database.

## The history is real

Every weight comes from the same progression engine that prescribes yours. The
generated lifters hit their targets and advance, miss and repeat, and stall three
times and deload — because the code deciding that is the code that decides it for
you, not a second copy that approximates it.

That is the whole reason this is worth having. Data generated against a second
implementation of the rules is data that can be wrong in exactly the ways you are
using it to check for, and a leaderboard ranking numbers no lifter could have
produced tells you nothing about whether the leaderboard works.

The same goes for the rest: sessions are given a plausible evening start and a
30–70 minute length, because duration drives session pace, the fastest-session
highlight and the ranking of a workout against its own history — and an install of
zero-second sessions shows those features working on nonsense. Reactions are chosen
from the same allowlist the API validates against. Comments are trimmed and within
the same cap. No lifter reacts to their own session, because the generator reads
through the feed query, which excludes the viewer's own.

### Each lifter has their own voice

Comments are **partitioned per persona** — no phrase belongs to two of them. With
one shared list, two lifters commenting on the same session could both say "nice
one", which reads as a single generator wearing several names rather than as several
people.

Each set is written in its own register, matched to how often that persona actually
comments: the reliable one who speaks rarely is terse to the point of curt, the
enthusiast is exclamatory, one is dryly sarcastic, another thinks in trends. The
register is the point — the difference has to survive being *read*, not just be
distinct as strings.

The binding is as persistent as the roster. Slot N always gets voice N, so the
account created at that slot keeps one voice across every run, every backfill and
every teardown-and-regenerate.

Two rules are enforced by tests rather than by care, because both are easy to break
by eye:

- **No phrase in two voices.** The sets are long enough that an accidental
  duplicate would be hard to spot. Applies across all three sets below, so a phrase
  cannot be both vague and templated either.
- **No phrase names a lift or a number.** One saying "nice 225" would have to agree
  with the session it hangs off, and one that did not would be the most obvious
  tell in the whole simulation. Keep new phrases vague.

### Some comments name the program or a lift

About **one comment in four** (`activity.MentionChance`) names something off the
session it is landing on — "I keep meaning to try StrongLifts 5x5", "no hiding on
Deadlift" — instead of vague approval. Specific is what makes a feed read like
people who looked at what they were commenting on; *always* specific is a mail
merge, which is worse than vague, so the rest stay general.

That does not loosen the rule above, it satisfies it from the other side. The rule
was never "be vague", it was **never assert anything you were not told**. A phrase
in `voices` is written blind — nobody had the session in view — so it can only
safely be vague. A phrase in `programVoices` or `exerciseVoices` carries one `%s`,
and the name that fills it is read off the session being commented on, so it cannot
disagree with it.

The material comes from `sessionMention` in `internal/api/activity.go`, and **a
persona may only name what the feed already showed it**:

- The program name is the one `ListFeedSessions` returned, which is **already masked**
  for this viewer. Where the masking fired, nothing is named — `Custom program` is a
  placeholder, not a programme, and admiring one by that name is both a tell and a lie.
- The lifts come from `ListMentionableSessionExercises`, which drops **custom
  exercises** (they belong to the lifter who made them, and `ListExercises` hides
  other people's) and lifts that were **not actually performed**. A generated comment
  is readable by the whole install, so naming a private exercise would be the feed's
  program-masking leak one surface further on.

The lookup costs a query, so it is only paid for the comments that are going to use
it: `MentionsSomething` is asked *before* the Mention is built. An empty Mention is
not an error — it means there was nothing safe to say — and the persona falls back
to its vague voice, as it does for a name too long to read as a sentence.

## What it does not do

**Nothing marks these accounts.** There is no column in the schema and no field in
any response saying an account was generated. They are ordinary accounts that
happen to have been created by a loop.

The consequence is that **clean-up works by re-deriving the roster that named
them** — a fixed list of eight people in `internal/activity`. That is why the
roster is required to be stable and append-only, and why there are tests that fail
if it stops being either. Change a name and the account carrying the old one
becomes unremovable from the panel, which is why a better pun is a reason to
*append* to that list rather than to edit it.

Those eight are written out in Go and the admin form's suggestions in TypeScript
(`src/ui/src/lib/punNames.ts`), with nothing keeping them in step. Deliberately:
the UI's list is long and grows whenever somebody thinks of one, this one is eight
and is pinned by the paragraph above. They tell the same joke; they are not one
list.

**An account that administers the install is never touched**, whatever it is
called. The delete refuses on `NOT is_admin`, so a roster name colliding with yours
cannot cost you your training history.

### How clean-up knows what it made

The roster gives clean-up its **candidates**; the password decides which of them
actually go.

Every generated account is created with one fixed password that nothing ever signs
in with. That makes the stored hash a proof of origin only the generator could have
produced — already in the database, costing no column, appearing on no wire. So a
clean-up lists the non-admin accounts whose usernames are on the roster, verifies
each one's hash against that password, and deletes only the matches.

The consequence is the one that matters: **a real lifter who happens to be named
after a persona survives.** They chose their own password, so their hash does not
verify, and they keep every session they ever logged. That collision is plausible
rather than contrived — the admin form suggests usernames from the same pun list
the personas are named out of, so the install itself may well have offered somebody
`judi.bench` — which is why it is worth closing properly rather than warning about.

An account that administers the install is never a candidate in the first place —
`NOT is_admin`, at both the lookup and the delete.

This is the one sanctioned exception to the rule that `GetUserForLogin` is the only
query reading `password_hash`. The hash here is evidence, not a credential: it is
verified in process, used as a boolean and discarded, and nothing derived from it is
returned. Both `db/queries/users.sql` and `db/queries/activity.sql` say so.

### Generated accounts cannot sign in

The flip side of using the password as proof is that the password is a constant in
this repository. So **login refuses it outright** — anyone who has read the source
would otherwise be able to authenticate as `judi.bench` and post comments and
reactions as them. The generator being owner-only says nothing about the login route;
that is a separate door and it is now shut.

The refusal is worded and timed exactly like a wrong password, so it does not
disclose which accounts are generated, and it is checked against the presented
credential rather than the stored hash — the only thing a generated account's hash
verifies is that one string, so anything else has already failed, and this costs no
extra work on a real login.

Changing `generatedPassword` orphans every account an earlier run created: their
hashes stop verifying, so a clean-up will leave them behind.

## Where it lives

| | |
|--|--|
| `src/api/internal/activity/` | Personas and decisions. No database, no HTTP — just "did they turn up", "did they hit their reps", "do they say something". Unit tested. |
| `src/api/internal/api/activity.go` | The runner and the handlers. Calls `prescribe`, and reuses `allowedReactions` and `maxCommentBody` rather than holding second copies. |
| `src/api/db/queries/activity.sql` | Four queries reachable only from here. Two of them can do things no ordinary request may — rewrite a session's clock, and delete accounts — which is why they are in a file of their own rather than mixed in with the rest. |
| `src/ui/src/routes/Astroturfing.svelte` | The screen. A heading and the panel — it exists so this stopped being a card on the accounts page. |
| `src/ui/src/lib/ActivityPanel.svelte` | The controls: backfill, live loop, daily schedule, clean-up. |
| `src/ui/src/lib/toast.svelte.ts` | The notice store, and general shell furniture rather than anything to do with generating — this was simply the first feature that needed it. |

## Authorisation

The endpoints sit inside the API's `/admin` subtree, which applies `requireAdmin`
to everything under it. They inherit the check from where they are mounted rather
than from anybody remembering to add it — the same property that subtree's own
comment exists to guarantee. A non-admin gets `403 admin_required`; anonymous gets
401.

They are always mounted. There is no separate switch at boot, so the capability is
present in every build and is guarded by the admin check alone.

## Bounds

Up to **8 lifters** (the roster's size) and **26 weeks** per backfill; ticks
between **5 seconds** and **an hour**. The panel reads those limits from the server
rather than hard-coding them, so its inputs cannot offer a number the endpoint
would refuse.

Backfill is synchronous. The bounds are what keep it fast enough for that to be
reasonable, rather than needing a job to poll.

## Re-running

Generating twice adds history to the lifters the first run made rather than failing
on a taken username, so you can deepen an install a few weeks at a time. The
summary reports accounts it **created**, which is why a second run says zero.

## One side effect worth knowing

Attendance needs a schedule to grade against, so generated lifters need their
program days given weekdays. `program_days` is **shared across the install** — one
row per day per program, for everybody — so this can only ever fill in a blank. A
day you have already scheduled is left exactly as you set it, which is what
`SetProgramDayWeekdayIfUnset` is for.

If you have eight lifters and eight programs they get one each and their schedules
do not collide. Beyond that they share, and two lifters on one program necessarily
train the same weekdays.
