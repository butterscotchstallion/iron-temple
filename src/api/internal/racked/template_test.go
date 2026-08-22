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

// The footer must not present a rate the report does not have. Most lifters have
// no scheduled weekdays, so most emails take the second branch.
func TestRenderEmailFooterMatchesTheAttendanceBasis(t *testing.T) {
	rep := marchReport(t)

	t.Run("no schedule reports frequency", func(t *testing.T) {
		rep.Attendance = Attendance{Basis: AttendanceNone, Actual: 12, SessionsPerWeek: 2.75}
		html, err := RenderEmail("Ada", rep)
		if err != nil {
			t.Fatalf("RenderEmail: %v", err)
		}
		if !strings.Contains(html, "2.8 sessions a week") {
			t.Error("footer does not report the training frequency")
		}
		if strings.Contains(html, "scheduled sessions") {
			t.Error("footer claims a rate against a schedule that does not exist")
		}
	})

	t.Run("a schedule reports a rate", func(t *testing.T) {
		rep.Attendance = Attendance{
			Basis: AttendanceWeekday, Expected: 13, Actual: 12, Rate: 0.923, SessionsPerWeek: 2.75,
		}
		html, err := RenderEmail("Ada", rep)
		if err != nil {
			t.Fatalf("RenderEmail: %v", err)
		}
		if !strings.Contains(html, "92% of your scheduled sessions") {
			t.Error("footer does not report the rate against the program")
		}
	})
}

// The bodyweight section appears only where there is a bodyweight, and quotes
// both ends and the delta — the email has no chart to show a trend with, so the
// numbers have to carry it.
func TestRenderEmailBodyweight(t *testing.T) {
	rep := marchReport(t)
	rep.Bodyweight = bodyweight([]WeighIn{
		{PerformedOn: day(2026, time.March, 2), WeightLb: 184},
		{PerformedOn: day(2026, time.March, 16), WeightLb: 181.4},
	})

	html, err := RenderEmail("Ada Lovelace", rep)
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}
	for _, want := range []string{"Bodyweight", "184", "181.4", "-2.6 lb"} {
		if !strings.Contains(html, want) {
			t.Fatalf("email is missing %q", want)
		}
	}
}

// A single reading has a number and no trend, so the section says the number and
// makes no claim about a change.
func TestRenderEmailBodyweightWithOneWeighIn(t *testing.T) {
	rep := marchReport(t)
	rep.Bodyweight = bodyweight([]WeighIn{{PerformedOn: day(2026, time.March, 2), WeightLb: 181}})

	html, err := RenderEmail("Ada Lovelace", rep)
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}
	if !strings.Contains(html, "Weighed in at") || !strings.Contains(html, "181") {
		t.Fatal("email does not report the single weigh-in")
	}
	if strings.Contains(html, "Start to end") {
		t.Fatal("email quotes a change from one reading")
	}
}

// Most lifters never fill the box in, and their recap should not carry an empty
// heading for a section with nothing under it.
func TestRenderEmailOmitsBodyweightWhenThereIsNone(t *testing.T) {
	html, err := RenderEmail("Ada Lovelace", marchReport(t))
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}
	if strings.Contains(html, "Bodyweight") {
		t.Fatal("email carries a bodyweight section for a period with no weigh-in")
	}
}

// The split line states the division and names the assistance rows, so a reader
// of the email can tell a lat pulldown from a squat the way a reader of the page
// can.
func TestRenderEmailReportsTheAssistanceSplit(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200)...)
	sets = append(sets, assist(mkSets(1, day(2026, time.March, 2), 9, "Barbell Curl", 3, 10, 40))...)

	html, err := RenderEmail("Ada Lovelace", Build(Input{
		Kind: PeriodMonth, Start: start, End: end, Sets: sets,
	}))
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}
	for _, want := range []string{"Main lifts", "assistance", "1 movement"} {
		if !strings.Contains(html, want) {
			t.Fatalf("email is missing %q", want)
		}
	}
}

// A lifter who does no assistance should not be told that all of their work was
// the program's. That is not news, it is the default.
func TestRenderEmailOmitsTheSplitWithoutAssistance(t *testing.T) {
	html, err := RenderEmail("Ada Lovelace", marchReport(t))
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}
	if strings.Contains(html, "Main lifts") {
		t.Fatal("email states a split for a lifter with no assistance work")
	}
}

func TestFormatWeightLb(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{181.4, "181.4"},
		// The trailing ".0" goes: a scale that read exactly 181 should say so.
		{181, "181"},
		{0, "0"},
	}
	for _, tc := range cases {
		if got := formatWeightLb(tc.in); got != tc.want {
			t.Fatalf("formatWeightLb(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSignedWeightLb(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{-2.6, "-2.6"},
		{1.5, "+1.5"},
		{2, "+2"},
		// Held steady is a real answer and reads as one, not as "+0".
		{0, "0"},
	}
	for _, tc := range cases {
		if got := signedWeightLb(tc.in); got != tc.want {
			t.Fatalf("signedWeightLb(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The recap's muscle section, which is the only part of the email whose most
// useful rows are the empty ones.
func TestRenderEmailNamesTheMuscleGaps(t *testing.T) {
	// marchReport's sets carry no muscle group, so tag a fresh set of them: a
	// month of squats and bench, and nothing else.
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))
	var sets []Set
	sets = append(sets,
		muscle(mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200), "legs")...)
	sets = append(sets,
		muscle(mkSets(2, day(2026, time.March, 9), 2, "Bench Press", 5, 5, 135), "chest")...)
	rep := Build(Input{Kind: PeriodMonth, Start: start, End: end, Sets: sets})

	html, err := RenderEmail("Ada Lovelace", rep)
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}

	if !strings.Contains(html, "What you trained") {
		t.Error("email is missing the muscle section")
	}
	// Every group has a row, trained or not — the untrained ones are the point.
	for _, group := range []string{"Legs", "Chest", "Back", "Shoulders", "Arms", "Core"} {
		if !strings.Contains(html, ">"+group+"</td>") {
			t.Errorf("email has no row for %q", group)
		}
	}
	// And the gaps are named in prose, not left to be inferred from a column of
	// empty bars.
	if !strings.Contains(html, "Nothing logged for back, shoulders, arms, core and other.") {
		t.Error("email does not name the untrained groups")
	}
}

// A period with no work has no muscle section at all — a recap that told a
// lifter they trained none of seven groups would be piling on.
func TestRenderEmailOmitsMusclesForAQuietPeriod(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))
	html, err := RenderEmail("Ada Lovelace",
		Build(Input{Kind: PeriodMonth, Start: start, End: end}))
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}
	if strings.Contains(html, "What you trained") {
		t.Error("an empty period rendered the muscle section")
	}
}

func TestJoinNames(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"core"}, "core"},
		{[]string{"core", "arms"}, "core and arms"},
		{[]string{"core", "arms", "chest"}, "core, arms and chest"},
	} {
		if got := joinNames(tc.in); got != tc.want {
			t.Errorf("joinNames(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
