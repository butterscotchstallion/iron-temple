// Package activity generates plausible training history for an install that
// needs some, so the screens built for a shared gym can be looked at without
// waiting three months for four people to fill them.
//
// Everything here is a DECISION and nothing here is a WRITE. The package holds
// personas, the roster they come from, and the small judgements a simulated
// lifter makes — did they turn up, did they hit their reps, do they say
// something. It touches no database and knows nothing about HTTP, which is what
// makes the interesting part testable: "this persona trains about three days in
// four" is a claim you can check in a millisecond, and a claim that would
// otherwise only be checkable by eye over a simulated month.
//
// The writing lives in internal/api, where it reuses the real prescription
// engine, the real reaction allowlist and the real comment rules rather than a
// second copy of any of them.
//
// # DETERMINISM
//
// Every decision takes an explicit *rand.Rand. Nothing reads a global source, so
// a run is reproducible from its seed — which matters twice: a surface that looks
// wrong can be regenerated exactly, and the roster can be re-derived later to
// find the accounts a previous run created.
package activity

import (
	"math/rand"
	"time"
)

// Persona is one simulated lifter's habits.
//
// Four numbers, all probabilities, because four is enough to make a roster look
// like people rather than like a loop. The point is not realism for its own sake
// — it is that the surfaces under test need VARIANCE. A leaderboard where
// everybody trained identically ranks nobody, an attendance board needs somebody
// who misses sessions, and the progression engine only shows its deload logic to
// a lifter who fails.
type Persona struct {
	Username    string
	DisplayName string

	// Consistency is the chance they train on a day they are scheduled for.
	// Below 1 by design: perfect attendance across a whole roster would leave the
	// attendance board flat and the streak board meaningless.
	Consistency float64
	// Grit is the chance a session goes well — every prescribed rep hit. A lifter
	// with low grit stalls and eventually deloads, which is the only way to see
	// the progression engine do its most interesting work.
	Grit float64
	// Sociability is the chance they react to another lifter's session they come
	// across.
	Sociability float64
	// Chattiness is the chance they comment on one. Deliberately lower than
	// Sociability for every persona in the roster: tapping an emoji is cheaper
	// than writing a sentence, and a feed where every session has three comments
	// reads as generated.
	Chattiness float64

	// voice is the phrases this persona alone says. Unexported because it is not a
	// knob — it is the persona's identity, assigned by Roster and fixed for the
	// life of the account that carries the name. Read through Comment.
	voice []string
}

// names is the roster personas are drawn from, in order.
//
// Lifting puns on famous names, eight of them, drawn from the same joke the admin
// area's "add someone" form suggests usernames from (src/ui/src/lib/punNames.ts).
// The install already tells that joke to the one person who can add an account;
// having the generator tell a different one meant a demo install populated with
// strangers, when the whole point of these accounts is that they are obviously
// furniture.
//
// Duplicated rather than shared, because a Go package cannot read a TypeScript
// const and the alternative — generating one from the other — is a build step in
// service of sixteen strings. The two lists agree in SPIRIT, not by mechanism: the
// UI's is long and grows freely, this one is eight and cannot. Adding a pun over
// there needs nothing done here.
//
// The order is fixed and the list is append-only in spirit: Roster(4) must keep
// returning the same four people across runs, because that is what lets a later
// teardown find the accounts an earlier backfill created without a marker column
// to look them up by. Renaming a slot strands the account carrying the old name —
// so a better pun is not a reason to edit this list, it is a reason to append.
//
// One consequence worth naming: the admin form suggests these very names, so a
// real lifter called "judi.bench" is now plausible rather than a freak accident.
// Teardown handles it — it verifies the password hash and never the name alone,
// see removeGeneratedLifters — and that check is what makes this list safe to draw
// from a suggestion box at all.
var names = []struct{ username, display string }{
	{"judi.bench", "Judi Bench"},
	{"dua.lats", "Dua Lats"},
	{"bulkie.eilish", "Bulkie Eilish"},
	{"clint.beastwood", "Clint Beastwood"},
	{"morgan.freeweight", "Morgan Freeweight"},
	{"seth.rowgen", "Seth Rowgen"},
	{"albert.gainstein", "Albert Gainstein"},
	{"keanu.heaves", "Keanu Heaves"},
}

// MaxRoster is how many personas exist. Asking for more yields this many rather
// than inventing names: an invented name could not be re-derived, so the account
// carrying it could never be torn down.
//
// A var rather than a const because len of a slice is not a constant expression.
// Read-only in practice — nothing assigns to it.
var MaxRoster = len(names)

// habit is the four-number shape each roster slot gets, cycled so that any n
// picks up a spread rather than four near-identical lifters.
//
// Hand-written rather than randomised from the seed: these ARE the variance the
// surfaces are being tested against, so they should be stable and legible —
// "slot 2 is the flaky one" is a useful thing to be able to rely on when a
// screen looks wrong.
var habits = []struct{ consistency, grit, sociability, chattiness float64 }{
	// The reliable one: turns up, mostly succeeds, says little.
	{0.95, 0.85, 0.35, 0.08},
	// The enthusiast: always there, always talking.
	{0.90, 0.70, 0.80, 0.35},
	// The flaky one: misses a third of their sessions, which is what gives the
	// attendance board something to rank and the streak board something to break.
	{0.65, 0.75, 0.45, 0.12},
	// The grinder: never misses, fails often, so the deload path gets exercised.
	{0.98, 0.45, 0.20, 0.05},
	{0.80, 0.80, 0.55, 0.20},
	{0.72, 0.60, 0.30, 0.10},
	{0.88, 0.90, 0.65, 0.25},
	{0.60, 0.55, 0.15, 0.04},
}

// Roster returns the first n personas, deterministically.
//
// n is clamped to [0, MaxRoster] rather than validated, so a caller that asks for
// twenty gets eight instead of an error — the endpoint above this bounds the
// request anyway, and a backfill that refused outright over a number it could
// safely interpret would be the less useful behaviour.
func Roster(n int) []Persona {
	if n < 0 {
		n = 0
	}
	if n > MaxRoster {
		n = MaxRoster
	}
	out := make([]Persona, 0, n)
	for i := range n {
		h := habits[i%len(habits)]
		out = append(out, Persona{
			Username:    names[i].username,
			DisplayName: names[i].display,
			Consistency: h.consistency,
			Grit:        h.grit,
			Sociability: h.sociability,
			Chattiness:  h.chattiness,
			// Slot N always gets voice N, which is what makes a persona's voice as
			// persistent as its name: the account created at this slot says the same
			// things across every run, and never says another persona's lines.
			voice: voices[i%len(voices)],
		})
	}
	return out
}

// Usernames is the roster's usernames for a given size.
//
// The teardown path's whole basis. These accounts carry no marker — nothing in
// the schema or on the wire says they were generated — so the only honest way to
// find them again is to ask the same function that named them.
func Usernames(n int) []string {
	roster := Roster(n)
	out := make([]string, 0, len(roster))
	for _, p := range roster {
		out = append(out, p.Username)
	}
	return out
}

// CatchUpWindow is how far back an unattended run will fill in.
//
// Seven days rather than racked's forty. The reporter's window is generous because
// a late email is still worth sending and there is at most one per period; this
// generates a day of training per day it covers, so a month of downtime would
// otherwise produce a month of sessions in one tick — a spike that looks nothing
// like a gym and takes a while to write.
//
// A week is the useful amount: a laptop closed over a weekend or a deploy on
// Tuesday catches up completely, and anything longer than that is an install
// nobody was looking at anyway.
const CatchUpWindow = 7 * 24 * time.Hour

// DueDays lists the days an unattended run should consider, oldest first.
//
// A pure function of the clock, exactly as racked.DuePeriods is, and for the same
// reason: whether a given day has ALREADY been generated is not decided here — that
// is generated_activity_runs' job — so this stays trivially testable across month
// and year boundaries without a database.
//
// Today is included. A day is generated as soon as it starts rather than once it
// has finished, because the point is an install that looks alive now; a lifter who
// trains on Tuesday morning should appear in the feed on Tuesday, not on Wednesday.
//
// Dates are returned at UTC midnight, which is how every date in this schema is
// held.
func DueDays(now time.Time, window time.Duration) []time.Time {
	if window < 0 {
		window = 0
	}
	// Truncated to a date in the caller's zone first, so "today" means the operator's
	// today rather than an instant. Rebuilt at UTC midnight afterwards because that
	// is the form a DATE column round-trips.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	days := int(window / (24 * time.Hour))
	out := make([]time.Time, 0, days+1)
	for i := days; i >= 0; i-- {
		out = append(out, today.AddDate(0, 0, -i))
	}
	return out
}

// Trains reports whether this persona turned up for a session they were
// scheduled for.
func (p Persona) Trains(rng *rand.Rand) bool { return rng.Float64() < p.Consistency }

// SessionGoesWell reports whether a session hits every prescribed rep. Decided
// once per session rather than per set, because a lifter who fails does so
// because the weight is too heavy that day, not independently on each set.
func (p Persona) SessionGoesWell(rng *rand.Rand) bool { return rng.Float64() < p.Grit }

// Reacts and Comments are asked per session a persona comes across.
func (p Persona) Reacts(rng *rand.Rand) bool   { return rng.Float64() < p.Sociability }
func (p Persona) Comments(rng *rand.Rand) bool { return rng.Float64() < p.Chattiness }

// SetReps is how many reps a persona logs against one prescribed set.
//
// On a session that went well, every set makes its target. On one that did not,
// the EARLY sets still make it and the last two fall short — which is how failure
// actually arrives under a linear programme, and which matters here because the
// progression engine reads "did every set hit its target" to decide whether the
// weight moves. A simulation that failed the first set and made the rest would
// stall the lift just the same but would look wrong on the recap.
//
// Never returns zero: a set logged at zero reps is not a short set, it is a set
// nobody performed, and the whole app treats those differently — they do not
// count toward volume and they keep a session out of the history page.
// setNumber and totalSets are plain ints while target and the result are int32.
// That is not inconsistency: target and the return value are reps, which is what
// the column stores, whereas the other two are positions in a list. Taking them as
// ints means the caller widens its int32 set number rather than narrowing a slice
// length, and widening is the direction that cannot overflow.
func (p Persona) SetReps(target int32, setNumber, totalSets int, wentWell bool, rng *rand.Rand) int32 {
	if target <= 1 {
		return max(target, 1)
	}
	if wentWell || setNumber <= totalSets-2 {
		return target
	}
	// One or two reps short, so a miss reads as a hard set rather than a collapse.
	// Branched rather than converted from rng.IntN: the arithmetic is the same and
	// this way there is no int-to-int32 narrowing for a reader — or a linter — to
	// have to convince themselves about.
	shortfall := int32(1)
	if rng.Intn(2) == 1 {
		shortfall = 2
	}
	if reps := target - shortfall; reps >= 1 {
		return reps
	}
	return 1
}

// voices is one phrase set per roster slot, in the same order as names.
//
// ONE VOICE PER PERSONA, AND NO PHRASE SHARED BETWEEN TWO. A single pooled list
// meant four lifters drawing from the same fifteen sentences, so a busy week
// collected "nice one" twice from different people — which reads as one generator
// wearing several names rather than as several people. Partitioning it is what
// makes a feed look like a conversation. The no-overlap rule is enforced by a test
// rather than by care: the sets are long enough that an accidental repeat is easy
// to introduce and hard to spot by eye.
//
// Each set is written in a recognisable register — terse, exuberant, dry, grim —
// so the difference survives being read rather than merely being distinct as
// strings. Register is matched to the persona's Chattiness: the ones who hardly
// ever comment get the shortest lines, because somebody who speaks once a month
// does not write paragraphs.
//
// NOTHING NAMES A LIFT OR A NUMBER. A comment saying "nice 225" would have to
// agree with the session it hangs off, and one that did not would be the most
// obvious tell in the whole simulation. Vague approval is both more realistic and
// safer, and it is the one rule to keep when adding phrases here.
var voices = [][]string{
	// judi.bench — reliable, says little. Understated to the point of curt.
	{
		"solid",
		"good work",
		"that'll do it",
		"steady",
		"no complaints there",
		"looks about right",
		"consistent as ever",
		"well held",
		"that's the way",
		"quietly impressive",
		"tidy",
		"nothing wasted there",
	},
	// dua.lats — the enthusiast. Loud, generous, exclamatory.
	{
		"absolutely flying!",
		"look at you go",
		"this is the best one yet surely",
		"okay that's just showing off",
		"enormous session",
		"you're on another level lately",
		"every single week, incredible",
		"I'm genuinely impressed",
		"that is a serious bit of work",
		"unreal consistency",
		"the grind is paying off big time",
		"hats off, that looked brutal",
	},
	// bulkie.eilish — flaky, warm, a little self-deprecating about it.
	{
		"meanwhile I skipped mine",
		"putting me to shame here",
		"love to see it",
		"wish I had your discipline",
		"this is motivating, genuinely",
		"right, I'm going tomorrow",
		"you always show up",
		"how do you keep it up?",
		"making me feel guilty in the best way",
		"respect for turning up",
		"one of us is doing it properly",
		"okay you've convinced me",
	},
	// clint.beastwood — the grinder who fails often. Grim, technical, no warmth.
	{
		"that looked heavy",
		"hard earned",
		"the last one was a fight",
		"no easy reps in there",
		"grim work",
		"that's where it gets difficult",
		"held together well",
		"bar speed dropped at the end",
		"through the sticking point",
		"that is the useful kind of hard",
		"nothing pretty about it",
		"earned every one",
	},
	// morgan.freeweight — encouraging, coach-like.
	{
		"great to see the progress",
		"you're building something here",
		"trust the process, it's working",
		"strong and controlled",
		"that's how it's done",
		"the consistency is the win",
		"keep exactly this up",
		"real progress this month",
		"form held right to the end",
		"proud of that one",
		"this is what patience looks like",
		"onwards",
	},
	// seth.rowgen — dry, wry, understated humour.
	{
		"well, that happened",
		"the bar lost that argument",
		"not bad for a Tuesday",
		"suspiciously easy looking",
		"showing the rest of us up again",
		"I'll pretend that's achievable",
		"someone's been eating properly",
		"rude, frankly",
		"noted, and resented",
		"the gym's least favourite visitor",
		"you've done this before, haven't you",
		"unnecessary but appreciated",
	},
	// albert.gainstein — analytical, thinks in trends and process.
	{
		"the trend on this is excellent",
		"nice clean progression",
		"that's a proper jump from last time",
		"consistent week over week",
		"the volume is really adding up",
		"good to see it moving again",
		"that curve is going the right way",
		"no stalling at all lately",
		"steady climb, no drama",
		"the numbers are doing what they should",
		"disciplined pacing",
		"exactly on schedule",
	},
	// keanu.heaves — barely comments at all. Telegraphic when they do.
	{
		"strong",
		"good",
		"yes",
		"big",
		"nice",
		"heavy",
		"clean",
		"sharp",
		"quality",
		"proper",
		"class",
		"more of that",
	},
}

// Comment is something this persona would say.
//
// A method rather than a package function, because what gets said depends on who
// is saying it: a persona carries its own phrases and never reaches for another's.
// That binding is as persistent as the roster — slot N always gets voice N, so the
// account named at slot N keeps one voice across every run and every backfill.
//
// A Persona built by hand rather than by Roster has no voice, which only ever
// happens in a test. It gets a phrase anyway rather than an empty string, because
// the API rejects a blank comment and a caller should not have to know that.
func (p Persona) Comment(rng *rand.Rand) string {
	if len(p.voice) == 0 {
		return "good work"
	}
	return p.voice[rng.Intn(len(p.voice))]
}

// Emoji picks a reaction from the set the caller offers.
//
// Takes the allowlist rather than holding one, because the authoritative set
// lives in internal/api next to the validation that enforces it — a second copy
// here would be a second thing to forget when it changes, and this package has no
// way to be right about it on its own.
func Emoji(allowed []string, rng *rand.Rand) string {
	if len(allowed) == 0 {
		return ""
	}
	return allowed[rng.Intn(len(allowed))]
}
