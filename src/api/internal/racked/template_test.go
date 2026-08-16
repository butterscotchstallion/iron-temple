package racked

import (
	"strings"
	"testing"
	"time"
)

// marchReport builds a populated recap for the template tests.
func marchReport(t *testing.T) Report {
	t.Helper()
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))

	var sets []Set
	sets = append(sets, finish(mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200), 50*time.Minute)...)
	sets = append(sets, finish(mkSets(2, day(2026, time.March, 9), 1, "Squat", 5, 5, 205), 45*time.Minute)...)
	sets = append(sets, finish(mkSets(3, day(2026, time.March, 16), 2, "Bench Press", 5, 5, 135), 40*time.Minute)...)

	return Build(Input{
		Kind:         PeriodMonth,
		Start:        start,
		End:          end,
		Sets:         sets,
		PreviousSets: mkSets(9, day(2026, time.February, 2), 1, "Squat", 5, 5, 100),
		Baseline:     Baseline{BestWeight: map[int32]float64{1: 195}, BestE1RM: map[int32]float64{1: 228}},
	})
}

func TestRenderEmail(t *testing.T) {
	html, err := RenderEmail("Ada Lovelace", marchReport(t))
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}

	for _, want := range []string{
		"Ada Lovelace lifted",
		"March 2026",
		"Squat",
		"Bench Press",
		"Most productive day",
		"personal record",
		"Iron Temple",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("email is missing %q", want)
		}
	}

	// Mail clients are not browsers: the layout has to survive without external
	// CSS, flexbox or SVG, so the recap's charts are table cells.
	for _, banned := range []string{"<svg", "display:flex", "<link", "class="} {
		if strings.Contains(html, banned) {
			t.Errorf("email contains %q, which mail clients handle badly", banned)
		}
	}
}

// Every optional section is absent from a quiet month, and the template must
// render rather than emit a section with nothing under it.
func TestRenderEmailEmptyPeriod(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))
	rep := Build(Input{Kind: PeriodMonth, Start: start, End: end})

	html, err := RenderEmail("Ada", rep)
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}
	for _, absent := range []string{"Moments", "Where the weight went", "Milestones", "Your lifter type"} {
		if strings.Contains(html, absent) {
			t.Errorf("empty recap still rendered the %q section", absent)
		}
	}
	if !strings.Contains(html, "March 2026") {
		t.Error("empty recap lost its period label")
	}
}

// Exercise names reach the template from user-controlled data, so the escaping
// html/template gives us is load-bearing, not incidental.
func TestRenderEmailEscapesNames(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))
	sets := mkSets(1, day(2026, time.March, 2), 1, `<script>alert("x")</script>`, 5, 5, 100)
	rep := Build(Input{Kind: PeriodMonth, Start: start, End: end, Sets: sets})

	html, err := RenderEmail(`Ada & "Bob"`, rep)
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}
	if strings.Contains(html, "<script>alert") {
		t.Error("an exercise name was interpolated as markup")
	}
	if !strings.Contains(html, "&amp;") {
		t.Error("the display name was not escaped")
	}
}

func TestSubjectNamesTheLifterAndPeriod(t *testing.T) {
	got := Subject("Ada Lovelace", marchReport(t))
	if got != "Racked: Ada Lovelace — March 2026" {
		t.Fatalf("Subject = %q", got)
	}
}

func TestBarPct(t *testing.T) {
	cases := []struct {
		value, max float64
		want       string
	}{
		{100, 100, "62.0"},
		{50, 100, "31.0"},
		{0, 100, "0"},
		{100, 0, "0"},
		{200, 100, "62.0"}, // clamped, never wider than its row
	}
	for _, tc := range cases {
		if got := barPct(tc.value, tc.max); got != tc.want {
			t.Errorf("barPct(%v, %v) = %q, want %q", tc.value, tc.max, got, tc.want)
		}
	}
}

func TestFormatEmailDuration(t *testing.T) {
	cases := map[time.Duration]string{
		48 * time.Minute:           "48m",
		time.Hour:                  "1h",
		time.Hour + 12*time.Minute: "1h 12m",
		0:                          "—",
		30 * time.Second:           "1m",
	}
	for in, want := range cases {
		if got := formatEmailDuration(in); got != want {
			t.Errorf("formatEmailDuration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSignedPercent(t *testing.T) {
	cases := map[float64]string{0.12: "+12%", -0.05: "-5%", 0: "0%"}
	for in, want := range cases {
		if got := signedPercent(in); got != want {
			t.Errorf("signedPercent(%v) = %q, want %q", in, got, want)
		}
	}
}

// sectionAfter returns the slice of html between a section heading and the next
// one, so a test can assert about one table rather than the whole email.
func sectionAfter(html, title string) string {
	start := strings.Index(html, title)
	if start < 0 {
		return ""
	}
	rest := html[start+len(title):]
	if end := strings.Index(rest, "font-size:12px;font-weight:700"); end >= 0 {
		return rest[:end]
	}
	return rest
}

// The lift table is capped; an email is skimmed once, and the lift that carried
// two percent of the month is not what the reader opened it for.
//
// Scoped to that table on purpose. The records list is deliberately NOT capped —
// seven personal records is a month worth reading about in full — so asserting
// against the whole email would be asserting the wrong thing.
func TestRenderEmailCapsTheLiftTable(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))
	var sets []Set
	names := []string{"Squat", "Bench Press", "Deadlift", "Overhead Press", "Barbell Row", "Pause Squat", "Chin Up"}
	for i, name := range names {
		sets = append(sets, mkSets(int32(i+1), day(2026, time.March, 2), int32(i+1), name, 1, 5, float64(200-i))...)
	}
	rep := Build(Input{Kind: PeriodMonth, Start: start, End: end, Sets: sets})

	html, err := RenderEmail("Ada", rep)
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}

	table := sectionAfter(html, "Where the weight went")
	if table == "" {
		t.Fatal("the lift table is missing entirely")
	}
	if !strings.Contains(table, "Squat") {
		t.Error("the lift table dropped the heaviest lift")
	}
	// The two lightest fall off.
	for _, dropped := range []string{"Chin Up", "Pause Squat"} {
		if strings.Contains(table, dropped) {
			t.Errorf("the lift table listed %q, past its cap", dropped)
		}
	}

	// ...but every record still gets its line.
	if records := sectionAfter(html, "personal record"); !strings.Contains(records, "Chin Up") {
		t.Error("the records list dropped a record; only the lift table is capped")
	}
}
