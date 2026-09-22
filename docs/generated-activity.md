# Generated activity

The screens built for a shared gym — the feed, the leaderboard, another lifter's
profile — need more than one lifter and more than one week of history before they
show anything worth looking at. This fills that in, so you can see them without
waiting three months for four people to do it for real.

Find it on **Account menu → Manage accounts**, above the roster. It is owner-only.

## What it does

**Generate history** creates lifters and walks forward through the weeks you ask
for, logging sessions on each one's scheduled days, then a pass of reactions and
comments. A few lifters over a few months takes a couple of seconds.

**Live** starts a loop where one lifter acts per tick — a session, a reaction or a
comment — so things arrive while you have a screen open. Useful for watching the
feed move; pointless for anything historical, which is what the backfill is for.

**Clean up** removes the generated lifters and everything of theirs.

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

## What it does not do

**Nothing marks these accounts.** There is no column in the schema and no field in
any response saying an account was generated. They are ordinary accounts that
happen to have been created by a loop.

The consequence is that **clean-up works by re-deriving the roster that named
them** — a fixed list of eight people in `internal/activity`. That is why the
roster is required to be stable and append-only, and why there are tests that fail
if it stops being either. Change a name and the account carrying the old one
becomes unremovable from the panel.

Two things are never touched:

- **An account that administers the install**, whatever it is called. The delete
  refuses on `NOT is_admin`, so a roster name colliding with yours cannot cost you
  your training history.
- **Anyone you created by hand**, because their name is not on the roster.

## Where it lives

| | |
|--|--|
| `src/api/internal/activity/` | Personas and decisions. No database, no HTTP — just "did they turn up", "did they hit their reps", "do they say something". Unit tested. |
| `src/api/internal/api/activity.go` | The runner and the handlers. Calls `prescribe`, and reuses `allowedReactions` and `maxCommentBody` rather than holding second copies. |
| `src/api/db/queries/activity.sql` | Four queries reachable only from here. Two of them can do things no ordinary request may — rewrite a session's clock, and delete accounts — which is why they are in a file of their own rather than mixed in with the rest. |

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
