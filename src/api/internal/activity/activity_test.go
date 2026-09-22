package activity

import (
	"math/rand"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// samePersona compares two personas field by field.
//
// Needed because Persona carries its voice as a slice, so == does not compile. The
// voice is included rather than skipped: it IS the identity being asserted as
// stable, so a comparison that ignored it would pass while the thing under test
// had changed.
func samePersona(a, b Persona) bool {
	return a.Username == b.Username &&
		a.DisplayName == b.DisplayName &&
		a.Consistency == b.Consistency &&
		a.Grit == b.Grit &&
		a.Sociability == b.Sociability &&
		a.Chattiness == b.Chattiness &&
		slices.Equal(a.voice, b.voice)
}

// A fixed source, so every assertion below is about the decision logic rather
// than about luck. PCG with constant seeds is reproducible across runs and across
// machines, which is the property the whole package is arranged around.
func fixed() *rand.Rand { return rand.New(rand.NewSource(1)) }

func TestRosterIsStableAcrossCalls(t *testing.T) {
	// The teardown path re-derives the roster to find accounts a previous run
	// created — there is no marker column to look them up by — so this is not a
	// tidiness assertion. If Roster stops being stable, generated accounts become
	// unremovable.
	first := Roster(4)
	second := Roster(4)
	if len(first) != 4 {
		t.Fatalf("Roster(4) returned %d personas", len(first))
	}
	for i := range first {
		if !samePersona(first[i], second[i]) {
			t.Errorf("persona %d differs between calls: %+v vs %+v", i, first[i], second[i])
		}
	}
}

// Roster(n) must be a prefix of Roster(n+1), or a teardown sized differently from
// the backfill that created the accounts would miss some and delete others.
func TestRosterGrowsByAppending(t *testing.T) {
	small, large := Roster(3), Roster(6)
	for i := range small {
		if !samePersona(small[i], large[i]) {
			t.Errorf("persona %d moved when the roster grew: %+v vs %+v", i, small[i], large[i])
		}
	}
}

func TestRosterClampsRatherThanInventingNames(t *testing.T) {
	// Invented names could not be re-derived, so they could never be torn down.
	if got := len(Roster(MaxRoster + 5)); got != MaxRoster {
		t.Errorf("Roster(%d) returned %d, want %d", MaxRoster+5, got, MaxRoster)
	}
	if got := len(Roster(-1)); got != 0 {
		t.Errorf("Roster(-1) returned %d personas, want 0", got)
	}
}

func TestRosterUsernamesAreUniqueAndValid(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Roster(MaxRoster) {
		if seen[p.Username] {
			t.Errorf("duplicate username %q — one account would collide with another", p.Username)
		}
		seen[p.Username] = true

		// The same rules the API enforces on any account (validateUsername): 3–32
		// characters of letters, digits, dot, underscore and hyphen. A persona that
		// failed them would be a backfill that dies on its first request.
		if len(p.Username) < 3 || len(p.Username) > 32 {
			t.Errorf("username %q is %d characters, outside 3–32", p.Username, len(p.Username))
		}
		for _, r := range p.Username {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
			if !ok {
				t.Errorf("username %q contains %q, which the API would reject", p.Username, r)
			}
		}
		if p.DisplayName == "" {
			t.Errorf("persona %q has no display name", p.Username)
		}
	}
}

func TestUsernamesMatchTheRoster(t *testing.T) {
	roster := Roster(5)
	names := Usernames(5)
	if len(names) != len(roster) {
		t.Fatalf("Usernames returned %d, roster has %d", len(names), len(roster))
	}
	for i := range roster {
		if names[i] != roster[i].Username {
			t.Errorf("entry %d: %q vs %q", i, names[i], roster[i].Username)
		}
	}
}

// The roster has to hold VARIANCE, which is the whole reason it exists: a
// leaderboard where everybody trained identically ranks nobody, and an attendance
// board needs somebody who misses sessions.
func TestRosterHoldsAMixOfHabits(t *testing.T) {
	roster := Roster(4)

	var perfect int
	for _, p := range roster {
		if p.Consistency >= 1 {
			perfect++
		}
		if p.Consistency < 0 || p.Consistency > 1 ||
			p.Grit < 0 || p.Grit > 1 ||
			p.Sociability < 0 || p.Sociability > 1 ||
			p.Chattiness < 0 || p.Chattiness > 1 {
			t.Errorf("persona %q has a probability outside 0–1: %+v", p.Username, p)
		}
		// Writing a sentence is more effort than tapping an emoji, and a feed where
		// every session carries three comments reads as generated.
		if p.Chattiness > p.Sociability {
			t.Errorf("persona %q comments more than it reacts (%.2f > %.2f)",
				p.Username, p.Chattiness, p.Sociability)
		}
	}
	if perfect == len(roster) {
		t.Error("every persona has perfect attendance, so the attendance board would be flat")
	}

	// Somebody has to fail often enough to reach a deload, or the progression
	// engine's most interesting path never runs.
	var fallible bool
	for _, p := range roster {
		if p.Grit <= 0.5 {
			fallible = true
		}
	}
	if !fallible {
		t.Error("no persona fails often enough to stall a lift and deload")
	}
}

// ---- the daily schedule's clock ----

// The window's worth of days plus today, oldest first. Oldest first matters: a
// catch-up generates history forward, and the progression engine reads a lifter's
// past to prescribe each session, so running the days backwards would prescribe
// from a future that had not happened yet.
func TestDueDaysCoversTheWindowEndingToday(t *testing.T) {
	now := time.Date(2026, 3, 17, 14, 30, 0, 0, time.UTC)
	got := DueDays(now, 3*24*time.Hour)

	want := []string{"2026-03-14", "2026-03-15", "2026-03-16", "2026-03-17"}
	if len(got) != len(want) {
		t.Fatalf("got %d days, want %d: %v", len(got), len(want), got)
	}
	for i, day := range got {
		if day.Format("2006-01-02") != want[i] {
			t.Errorf("day %d is %s, want %s", i, day.Format("2006-01-02"), want[i])
		}
		// Every date in this schema is held at UTC midnight, which is the form a
		// DATE column round-trips.
		if day.Hour()+day.Minute()+day.Second()+day.Nanosecond() != 0 {
			t.Errorf("day %d is not midnight: %s", i, day)
		}
		if day.Location() != time.UTC {
			t.Errorf("day %d is not UTC: %s", i, day.Location())
		}
	}
}

// Today is included rather than waiting for the day to finish — the point is an
// install that looks alive NOW, so a lifter who trains on Tuesday morning should
// appear in the feed on Tuesday.
func TestDueDaysIncludesToday(t *testing.T) {
	now := time.Date(2026, 3, 17, 0, 1, 0, 0, time.UTC)
	got := DueDays(now, 0)
	if len(got) != 1 {
		t.Fatalf("a zero window should yield today alone, got %v", got)
	}
	if got[0].Format("2006-01-02") != "2026-03-17" {
		t.Errorf("got %s, want today", got[0].Format("2006-01-02"))
	}
}

// The reason this is a pure function of the clock rather than a query: month and
// year boundaries are where date arithmetic goes wrong, and they are free to test
// here.
func TestDueDaysCrossesMonthAndYearBoundaries(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want []string
	}{
		{
			"across a month end",
			time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC),
			[]string{"2026-02-28", "2026-03-01", "2026-03-02"},
		},
		{
			"across a year end",
			time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
			[]string{"2025-12-30", "2025-12-31", "2026-01-01"},
		},
		{
			// 2028 is a leap year, so the 29th exists and must not be skipped.
			"across a leap day",
			time.Date(2028, 3, 1, 9, 0, 0, 0, time.UTC),
			[]string{"2028-02-28", "2028-02-29", "2028-03-01"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DueDays(tc.now, 2*24*time.Hour)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d days, want %d: %v", len(got), len(tc.want), got)
			}
			for i, day := range got {
				if day.Format("2006-01-02") != tc.want[i] {
					t.Errorf("day %d is %s, want %s", i, day.Format("2006-01-02"), tc.want[i])
				}
			}
		})
	}
}

// The window bounds the catch-up, which is the point of having one: a month of
// downtime must not produce a month of training in a single tick.
func TestDueDaysIsBoundedByItsWindow(t *testing.T) {
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	if got := len(DueDays(now, CatchUpWindow)); got != 8 {
		t.Errorf("the default window yielded %d days, want 8 (a week plus today)", got)
	}
	// A negative window is treated as none rather than panicking on a loop that
	// counts down from below zero.
	if got := DueDays(now, -48*time.Hour); len(got) != 1 {
		t.Errorf("a negative window yielded %d days, want today alone", len(got))
	}
}

// The time of day must not change which days come back — only the date does. A
// scheduler ticking hourly asks this question over and over, and an answer that
// drifted through the day would generate a day twice or skip one.
func TestDueDaysIgnoresTheTimeOfDay(t *testing.T) {
	var first []string
	for _, hour := range []int{0, 6, 13, 23} {
		now := time.Date(2026, 3, 17, hour, 45, 0, 0, time.UTC)
		var got []string
		for _, day := range DueDays(now, CatchUpWindow) {
			got = append(got, day.Format("2006-01-02"))
		}
		if first == nil {
			first = got
			continue
		}
		if !slices.Equal(first, got) {
			t.Errorf("hour %d gave %v, want %v", hour, got, first)
		}
	}
}

// ---- decisions ----

// The probabilities have to actually govern the outcome. Asserted over many draws
// rather than one, with loose bounds — this is a test that the number is wired to
// the decision, not that the RNG is uniform.
func TestDecisionsFollowTheirProbabilities(t *testing.T) {
	const draws = 4000
	cases := []struct {
		name  string
		p     Persona
		call  func(Persona, *rand.Rand) bool
		want  float64
		field string
	}{
		{"trains", Persona{Consistency: 0.25}, Persona.Trains, 0.25, "Consistency"},
		{"trains always", Persona{Consistency: 1}, Persona.Trains, 1, "Consistency"},
		{"trains never", Persona{Consistency: 0}, Persona.Trains, 0, "Consistency"},
		{"session", Persona{Grit: 0.75}, Persona.SessionGoesWell, 0.75, "Grit"},
		{"reacts", Persona{Sociability: 0.5}, Persona.Reacts, 0.5, "Sociability"},
		{"comments", Persona{Chattiness: 0.1}, Persona.Comments, 0.1, "Chattiness"},
	}

	for _, tc := range cases {
		rng := fixed()
		var yes int
		for range draws {
			if tc.call(tc.p, rng) {
				yes++
			}
		}
		got := float64(yes) / draws
		if diff := got - tc.want; diff > 0.05 || diff < -0.05 {
			t.Errorf("%s: %.3f of draws, want about %.2f (%s)", tc.name, got, tc.want, tc.field)
		}
	}
}

// A session that went well hits every target. The progression engine reads
// exactly this to decide whether the weight moves, so a simulation that fell short
// on a good session would never advance a lift.
func TestSetRepsHitsEveryTargetOnAGoodSession(t *testing.T) {
	p := Roster(1)[0]
	rng := fixed()
	for set := 1; set <= 5; set++ {
		if got := p.SetReps(5, set, 5, true, rng); got != 5 {
			t.Errorf("set %d of a good session logged %d reps, want 5", set, got)
		}
	}
}

// On a bad session the EARLY sets still make it and the last two fall short, which
// is how failure arrives under a linear programme. Failing the first set and
// making the rest would stall the lift just the same but would look wrong on the
// recap.
func TestSetRepsFailsLateNotEarly(t *testing.T) {
	p := Roster(1)[0]
	rng := fixed()
	const total = 5
	for set := 1; set <= total; set++ {
		got := p.SetReps(5, set, total, false, rng)
		switch {
		case set <= total-2:
			if got != 5 {
				t.Errorf("set %d should still make its target, logged %d", set, got)
			}
		default:
			if got >= 5 {
				t.Errorf("set %d should fall short, logged %d", set, got)
			}
			if got < 1 {
				t.Errorf("set %d logged %d reps — a set at zero is one nobody performed", set, got)
			}
		}
	}
}

// Zero reps is not a short set, it is a set nobody did: the app keeps those out of
// volume and out of the history page entirely. No combination of inputs may
// produce one.
func TestSetRepsIsNeverZero(t *testing.T) {
	rng := fixed()
	for _, p := range Roster(MaxRoster) {
		for _, target := range []int32{0, 1, 2, 3, 5, 8, 12} {
			for _, wentWell := range []bool{true, false} {
				for set := 1; set <= 5; set++ {
					if got := p.SetReps(target, set, 5, wentWell, rng); got < 1 {
						t.Fatalf("target %d, set %d, wentWell=%v logged %d reps",
							target, set, wentWell, got)
					}
				}
			}
		}
	}
}

// A single-rep target has nowhere to fall short to.
func TestSetRepsHandlesATinyTarget(t *testing.T) {
	p := Roster(1)[0]
	rng := fixed()
	if got := p.SetReps(1, 5, 5, false, rng); got != 1 {
		t.Errorf("a target of 1 logged %d reps, want 1", got)
	}
}

// ---- what they say ----

// Every persona always has something to say, and it is always acceptable to the
// API — which rejects a blank body and caps the length.
func TestEveryPersonaSaysSomethingAcceptable(t *testing.T) {
	rng := fixed()
	for _, p := range Roster(MaxRoster) {
		for range 100 {
			got := p.Comment(rng)
			if got == "" {
				t.Fatalf("%s returned an empty comment, which the API rejects as blank", p.Username)
			}
			// Counted in runes, the same way maxCommentBody counts.
			if len([]rune(got)) > 256 {
				t.Errorf("%s said something longer than the API accepts: %q", p.Username, got)
			}
			if got != strings.TrimSpace(got) {
				t.Errorf("%s said %q, which is not trimmed and would be stored untidy", p.Username, got)
			}
		}
	}
}

// THE REASON VOICES ARE PARTITIONED AT ALL.
//
// With one pooled list, two lifters commenting on the same session could both say
// "nice one" — which reads as one generator wearing several names rather than as
// several people. No phrase may belong to two personas, and the sets are long
// enough that an accidental duplicate is easy to introduce and hard to spot by eye,
// so it is asserted rather than trusted.
func TestNoTwoPersonasShareAPhrase(t *testing.T) {
	owner := map[string]string{}
	for _, p := range Roster(MaxRoster) {
		for _, phrase := range p.voice {
			if first, seen := owner[phrase]; seen {
				t.Errorf("%q belongs to both %s and %s", phrase, first, p.Username)
				continue
			}
			owner[phrase] = p.Username
		}
	}
}

// A persona draws only from its own set, never another's.
func TestAPersonaOnlySaysItsOwnLines(t *testing.T) {
	roster := Roster(MaxRoster)
	rng := fixed()
	for _, p := range roster {
		own := map[string]bool{}
		for _, phrase := range p.voice {
			own[phrase] = true
		}
		for range 200 {
			if got := p.Comment(rng); !own[got] {
				t.Fatalf("%s said %q, which is not in its own voice", p.Username, got)
			}
		}
	}
}

// Each voice needs enough in it that a persona is not visibly looping, and the
// whole corpus needs to be big enough that a populated feed reads as varied.
func TestVoicesAreLargeEnoughToNotRepeat(t *testing.T) {
	total := 0
	for _, p := range Roster(MaxRoster) {
		if len(p.voice) < 8 {
			t.Errorf("%s has only %d phrases, which will visibly loop", p.Username, len(p.voice))
		}
		// A duplicate inside one voice wastes a slot and makes a repeat likelier.
		seen := map[string]bool{}
		for _, phrase := range p.voice {
			if seen[phrase] {
				t.Errorf("%s lists %q twice", p.Username, phrase)
			}
			seen[phrase] = true
		}
		total += len(p.voice)
	}
	if total < 60 {
		t.Errorf("the whole corpus is only %d phrases", total)
	}
}

// Nothing may name a lift or a number. A comment saying "nice 225" would have to
// agree with the session it hangs off, and one that did not would be the most
// obvious tell in the simulation — so this is a rule about the DATA being
// plausible, not about tidiness.
func TestNoPhraseNamesALiftOrANumber(t *testing.T) {
	// Word-boundary matched, not substring. A naive Contains flags "impressive"
	// for "press" and "tomorrow" for "row", which would make this test reject
	// perfectly good English.
	lifts := regexp.MustCompile(`(?i)\b(squat|bench|deadlift|press|row|curl|lunge|chin|dip)\b`)
	for _, p := range Roster(MaxRoster) {
		for _, phrase := range p.voice {
			if lifts.MatchString(phrase) {
				t.Errorf("%s says %q, which names a lift", p.Username, phrase)
			}
			if strings.ContainsAny(phrase, "0123456789") {
				t.Errorf("%s says %q, which contains a number", p.Username, phrase)
			}
		}
	}
}

// A Persona built by hand has no voice, which only happens in a test. It must
// still produce something, because the API rejects a blank comment.
func TestAVoicelessPersonaStillSaysSomething(t *testing.T) {
	if got := (Persona{}).Comment(fixed()); got == "" {
		t.Error("a persona with no voice returned an empty comment")
	}
}

func TestEmojiComesFromTheCallersAllowlist(t *testing.T) {
	// The authoritative set lives next to the validation that enforces it, so this
	// package must never hold its own copy — it is handed one.
	allowed := []string{"💪", "🔥"}
	rng := fixed()
	for range 100 {
		got := Emoji(allowed, rng)
		if got != "💪" && got != "🔥" {
			t.Fatalf("Emoji returned %q, which is not in the allowlist", got)
		}
	}
}

func TestEmojiHandlesAnEmptyAllowlist(t *testing.T) {
	if got := Emoji(nil, fixed()); got != "" {
		t.Errorf("Emoji with no allowlist returned %q, want an empty string", got)
	}
}
