package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

// The gym setup: what the bar weighs, what plates are in the rack, and where a
// lift starts. All three exist because the app used to guess at them — a 45 lb
// bar and an unbounded plate set compiled into the UI bundle, and seeded
// starting weights that assumed the same bar. On an install whose bar is 80 lb
// that produced first sessions nobody could load.

// A fresh account owns the standard rack. This is what makes an *empty*
// inventory mean "owns no plates" rather than "never configured" — if
// registration left the table empty, the two would be indistinguishable and the
// loader could not honour either.
func TestNewAccountStartsWithABarAndAStandardRack(t *testing.T) {
	e := expect(t)

	me := e.GET("/me").Expect().Status(http.StatusOK).JSON().Object()
	me.Value("barWeightLb").Number().Gt(0)

	plates := me.Value("plates").Array()
	plates.NotEmpty()

	// Heaviest first: the order the greedy loader reads them in.
	var prev float64
	for i := 0; i < int(plates.Length().Raw()); i++ {
		p := plates.Value(i).Object()
		lb := p.Value("plateLb").Number().Raw()
		p.Value("pairs").Number().Gt(0)
		if i > 0 && lb >= prev {
			t.Fatalf("plates are not heaviest-first: %v followed %v", lb, prev)
		}
		prev = lb
	}
}

func TestGymSetupRoundTrips(t *testing.T) {
	e := expect(t)

	updated := e.PATCH("/me").
		WithJSON(map[string]any{
			"barWeightLb": 80,
			"plates": []map[string]any{
				{"plateLb": 45, "pairs": 2},
				{"plateLb": 10, "pairs": 1},
			},
		}).
		Expect().Status(http.StatusOK).JSON().Object()

	updated.Value("barWeightLb").Number().IsEqual(80)
	updated.Value("plates").Array().Length().IsEqual(2)

	// And it survives a re-read, so it was stored rather than echoed.
	me := e.GET("/me").Expect().Status(http.StatusOK).JSON().Object()
	me.Value("barWeightLb").Number().IsEqual(80)
	first := me.Value("plates").Array().Value(0).Object()
	first.Value("plateLb").Number().IsEqual(45)
	first.Value("pairs").Number().IsEqual(2)

	// Restore the standard rack so later tests see a normal gym.
	t.Cleanup(func() {
		e.PATCH("/me").WithJSON(map[string]any{
			"barWeightLb": 45,
			"plates": []map[string]any{
				{"plateLb": 45, "pairs": 2}, {"plateLb": 35, "pairs": 2},
				{"plateLb": 25, "pairs": 2}, {"plateLb": 10, "pairs": 2},
				{"plateLb": 5, "pairs": 2}, {"plateLb": 2.5, "pairs": 2},
			},
		}).Expect().Status(http.StatusOK)
	})
}

// The inventory is replaced whole, not patched. A lifter who sells their 35s
// sends the rack without them, and they must actually go — a merge would leave
// a plate they no longer own still loadable.
func TestPlatesAreReplacedWholeAndMayBeEmptied(t *testing.T) {
	e := expect(t)
	t.Cleanup(func() {
		e.PATCH("/me").WithJSON(map[string]any{
			"plates": []map[string]any{
				{"plateLb": 45, "pairs": 2}, {"plateLb": 35, "pairs": 2},
				{"plateLb": 25, "pairs": 2}, {"plateLb": 10, "pairs": 2},
				{"plateLb": 5, "pairs": 2}, {"plateLb": 2.5, "pairs": 2},
			},
		}).Expect().Status(http.StatusOK)
	})

	e.PATCH("/me").
		WithJSON(map[string]any{"plates": []map[string]any{{"plateLb": 45, "pairs": 3}}}).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("plates").Array().Length().IsEqual(1)

	// An empty rack is a thing a lifter is allowed to own, and is not read as
	// "leave it alone".
	e.PATCH("/me").
		WithJSON(map[string]any{"plates": []map[string]any{}}).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("plates").Array().IsEmpty()

	// Omitting the field entirely is what leaves it alone.
	e.PATCH("/me").
		WithJSON(map[string]any{"displayName": "Unchanged Rack"}).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("plates").Array().IsEmpty()
}

func TestGymSetupValidation(t *testing.T) {
	e := expect(t)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"zero bar", map[string]any{"barWeightLb": 0}},
		{"absurd bar", map[string]any{"barWeightLb": 500}},
		{"zero plate", map[string]any{"plates": []map[string]any{{"plateLb": 0, "pairs": 1}}}},
		{"zero pairs", map[string]any{"plates": []map[string]any{{"plateLb": 45, "pairs": 0}}}},
		{"repeated denomination", map[string]any{"plates": []map[string]any{
			{"plateLb": 45, "pairs": 1}, {"plateLb": 45, "pairs": 2},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e.PATCH("/me").WithJSON(tc.body).Expect().Status(http.StatusBadRequest)
		})
	}
}

// A baseline decides where a lift starts and nothing after it: it displaces the
// program's seeded starting weight, and the seed is only read while the lift has
// no history. This is the fix for seeds that assume a 45 lb bar on an install
// that owns an 80 lb one.
func TestBaselineOverridesTheSeededStartingWeight(t *testing.T) {
	e := expect(t)
	programID, dayID := programAndFirstDay(e, "StrongLifts 5x5")

	before := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("exercises").Array().Value(0).Object()
	exerciseID := int(before.Value("exerciseId").Number().Raw())

	// Only meaningful on a lift with no history, which is what "start" means.
	if before.Value("progression").Object().Value("status").String().Raw() != "start" {
		t.Skip("the first lift already has history in this run; nothing to baseline")
	}

	e.PUT(fmt.Sprintf("/me/baselines/%d", exerciseID)).
		WithJSON(map[string]any{"weightLb": 135}).
		Expect().Status(http.StatusNoContent)
	t.Cleanup(func() {
		e.DELETE(fmt.Sprintf("/me/baselines/%d", exerciseID)).Expect().Status(http.StatusNoContent)
	})

	e.GET("/me/baselines").Expect().Status(http.StatusOK).
		JSON().Array().Length().IsEqual(1)

	after := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("exercises").Array().Value(0).Object()
	after.Value("weightLb").Number().IsEqual(135)

	// And a session started from it materializes at the baseline, not the seed.
	created := startSession(t, e, dayID)
	created.Value("sets").Array().Value(0).Object().Value("weightLb").Number().IsEqual(135)
}

func TestBaselineValidationAndUnknownLifts(t *testing.T) {
	e := expect(t)

	e.PUT("/me/baselines/999999").
		WithJSON(map[string]any{"weightLb": 100}).
		Expect().Status(http.StatusNotFound)

	e.PUT("/me/baselines/1").
		WithJSON(map[string]any{"weightLb": -5}).
		Expect().Status(http.StatusBadRequest)

	e.PUT("/me/baselines/1").
		WithJSON(map[string]any{}).
		Expect().Status(http.StatusBadRequest)

	// Clearing one that was never set says so rather than pretending.
	e.DELETE("/me/baselines/999999").Expect().Status(http.StatusNotFound)

	// 0 is a legitimate baseline: an empty bar is where a press starts.
	e.PUT("/me/baselines/1").
		WithJSON(map[string]any{"weightLb": 0}).
		Expect().Status(http.StatusNoContent)
	e.DELETE("/me/baselines/1").Expect().Status(http.StatusNoContent)
}

// Gym setup is per-lifter state, and the rule this schema applies everywhere
// else is easier to keep than to remember exceptions to.
func TestGymSetupIsPrivateToItsOwner(t *testing.T) {
	e := expect(t)
	// createUser inserts directly, so this account has no user_gym row at all —
	// which also exercises GetBarWeight's fallback for an account that has never
	// opened the setup screen.
	other := expectAs(t, createUser(t, "rackmate", "Rack Mate", "correct horse battery"))

	e.PATCH("/me").
		WithJSON(map[string]any{"barWeightLb": 33}).
		Expect().Status(http.StatusOK)
	t.Cleanup(func() {
		e.PATCH("/me").WithJSON(map[string]any{"barWeightLb": 45}).
			Expect().Status(http.StatusOK)
	})

	// The second account keeps its own bar, not the first one's.
	if bar := other.GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().Value("barWeightLb").Number().Raw(); bar == 33 {
		t.Fatal("one lifter's bar weight was visible to another")
	}
}
