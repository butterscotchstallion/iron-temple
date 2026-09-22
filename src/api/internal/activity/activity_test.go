package activity

import (
	"math/rand/v2"
	"testing"
)

// A fixed source, so every assertion below is about the decision logic rather
// than about luck. PCG with constant seeds is reproducible across runs and across
// machines, which is the property the whole package is arranged around.
func fixed() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

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
		if first[i] != second[i] {
			t.Errorf("persona %d differs between calls: %+v vs %+v", i, first[i], second[i])
		}
	}
}

// Roster(n) must be a prefix of Roster(n+1), or a teardown sized differently from
// the backfill that created the accounts would miss some and delete others.
func TestRosterGrowsByAppending(t *testing.T) {
	small, large := Roster(3), Roster(6)
	for i := range small {
		if small[i] != large[i] {
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

func TestCommentIsAlwaysSomething(t *testing.T) {
	rng := fixed()
	seen := map[string]bool{}
	for range 200 {
		got := Comment(rng)
		if got == "" {
			t.Fatal("Comment returned an empty string, which the API rejects as blank")
		}
		// The cap the API enforces on any comment, counted the same way.
		if len([]rune(got)) > 256 {
			t.Errorf("comment %q is longer than the API accepts", got)
		}
		seen[got] = true
	}
	// A feed where every comment is the same sentence is worse than no comments.
	if len(seen) < 5 {
		t.Errorf("only %d distinct phrases over 200 draws", len(seen))
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
