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
}

// names is the roster personas are drawn from, in order.
//
// Ordinary-sounding and deliberately not famous. A list of physicists and
// computer scientists would be a tell, and these accounts are meant to look like
// the household they are standing in for rather than like a fixture.
//
// The order is fixed and the list is append-only in spirit: Roster(4) must keep
// returning the same four people across runs, because that is what lets a later
// teardown find the accounts an earlier backfill created without a marker column
// to look them up by.
var names = []struct{ username, display string }{
	{"mara.quinn", "Mara Quinn"},
	{"dev.oyelaran", "Dev Oyelaran"},
	{"tess.haraldsen", "Tess Haraldsen"},
	{"otto.brandt", "Otto Brandt"},
	{"nell.ferreira", "Nell Ferreira"},
	{"sunil.raghavan", "Sunil Raghavan"},
	{"ivy.kowalczyk", "Ivy Kowalczyk"},
	{"bram.delacroix", "Bram Delacroix"},
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

// phrases is what a simulated lifter says.
//
// Short, and about the training rather than about each other, because a comment
// that named a lift or a number would have to agree with the session it hangs
// off — and one that did not would be the most obvious tell in the whole
// simulation. Vague approval is both more realistic and safer.
var phrases = []string{
	"strong work",
	"that last set looked heavy",
	"nice one",
	"big session",
	"beast mode",
	"you make it look easy",
	"solid",
	"that's a PR surely",
	"good grinding",
	"impressive",
	"keep it rolling",
	"respect",
	"how did that feel?",
	"clean reps",
	"tough day but you got it",
}

// Comment picks something to say.
func Comment(rng *rand.Rand) string { return phrases[rng.Intn(len(phrases))] }

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
