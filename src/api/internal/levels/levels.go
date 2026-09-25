// Package levels turns a count of training sessions into a level a lifter wears.
//
// Everything here is a pure function of its input. The package holds no database
// handle and imports no pgtype, for internal/racked's reason: the arithmetic is
// the part that has to be right, and it is worth being able to test it without a
// Postgres in the room.
//
// # WHY THE CURVE ESCALATES
//
// Each level costs more than the one before it — level L costs (L-1) x 100 XP, so
// reaching it has cost 50*L*(L-1) in total. The alternative was a flat cost per
// level, and it is worse at both ends of a lifter's time here. A newcomer needs the
// badge to move in their first week or it reads as decoration rather than a record;
// a lifter three years in needs their number to mean something a newcomer's cannot
// cheaply match. A flat curve gives up on the second to buy nothing on the first,
// because a level that always costs five sessions never says more than the session
// count it is standing in for.
//
// In sessions, at 100 XP each: level 2 is 1 session, level 5 is 10, level 12 is 66,
// level 20 is 190, level 30 is 435.
//
// # WHAT A LEVEL DELIBERATELY DOES NOT MEASURE
//
// Not strength, not volume, not intensity. A session is worth 100 XP whether it was
// twenty sets or three, because the question the badge answers beside a name is "has
// this person been showing up", and that is the one claim a newcomer can read off a
// roster without knowing anything about the lifter. Weighting by volume would make
// the number partly a statement about how strong somebody already is, which the
// leaderboard and the crowns already say, and say better.
//
// The consequence is that the number is farmable by anyone who wants to bother — a
// single logged set, twelve hours apart, earns what a full workout earns. That is
// accepted. See the qualifying rule in db/queries/levels.sql for the one abuse that
// is actually guarded against, which is the session containing no work at all.
package levels

// XPPerSession is what one qualifying session is worth.
//
// A round number rather than a tuned one, and load-bearing only in that the whole
// curve is expressed in multiples of it: XP is always a session count times this, so
// any figure this package returns divides by it exactly.
const XPPerSession = 100

// Progress is where one lifter stands: the level they have reached, and how far
// into it they are.
//
// All four figures are computed here rather than leaving the client to derive the
// last two from XP, because deriving them means reimplementing the curve — and a
// second implementation in TypeScript is a thing that can disagree with this one.
// The header's hover card draws a bar from XPIntoLevel over XPForNextLevel and does
// no arithmetic at all.
type Progress struct {
	// Level is the level reached. Never below 1: an account that has never
	// trained is level 1 rather than level 0, because the badge is drawn from
	// the day somebody joins and a zero beside a name reads as an error.
	Level int
	// XP is the lifetime total.
	XP int
	// XPIntoLevel is how much of the current level has been earned — 0 on the
	// session that reached it.
	XPIntoLevel int
	// XPForNextLevel is what the whole of the current level costs, NOT what is
	// left to pay. It is the denominator of a progress bar, and sending the
	// remainder instead would make every caller reconstruct it.
	XPForNextLevel int
}

// totalFor is the lifetime XP needed to have reached level l.
//
// The closed form of "level L costs (L-1) x 100": summing that from 2 to L gives
// 100 * L*(L-1)/2, which is 50*L*(L-1) without the division and so without any
// question about where it rounds.
func totalFor(l int) int { return 50 * l * (l - 1) }

// FromSessions is the whole feature: qualifying sessions in, standing out.
//
// The level is found by climbing rather than by inverting the curve. The inverse
// exists and is exact in real arithmetic — floor((1+sqrt(1+8n))/2) — and it agrees
// with this for every session count from 0 to 200,000, which was checked rather
// than assumed. It is not used anyway, because it lands precisely ON a boundary at
// every threshold, which is the one place a float has room to be wrong, and its
// correctness would then rest on an argument about precision that this loop does
// not need to make. Levels grow as the square root of the session count, so the
// climb runs a few hundred times for a lifter who will never exist.
func FromSessions(n int) Progress {
	// A negative count is not reachable from a COUNT(*), but the zero value of an
	// int is, and clamping here means no caller has to think about it.
	if n < 0 {
		n = 0
	}

	xp := n * XPPerSession

	level := 1
	for totalFor(level+1) <= xp {
		level++
	}

	return Progress{
		Level:          level,
		XP:             xp,
		XPIntoLevel:    xp - totalFor(level),
		XPForNextLevel: level * XPPerSession,
	}
}
