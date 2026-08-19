package racked

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"
)

// The recap email.
//
// Table layout with inline styles, in the same visual idiom as the CI failure
// card in scripts/ci-notify.sh, because mail clients are not browsers: no
// external stylesheet, no flexbox or grid, no SVG. The charts on the page
// become table rows with percentage-width cells here — a bar chart is one of
// the few things HTML mail renders honestly.
//
// html/template rather than string concatenation so a lift named with an
// ampersand cannot break the markup, or worse, inject into it. Every value
// below is user-supplied by way of an exercise name or a display name.

// barWidth is how wide the widest bar in a table may be, as a percentage of the
// row. Short of 100 so the value beside it always has room.
const barWidth = 62.0

var emailTemplate = template.Must(template.New("racked").Funcs(template.FuncMap{
	"lb":       formatLb,
	"weight":   formatWeightLb,
	"signedLb": signedWeightLb,
	"pct":      formatEmailPercent,
	"dur":      formatEmailDuration,
	"date":     formatEmailDate,
	"barPct":   barPct,
	"plural":   plural,
	"weekday":  weekdayName,
	"signed":   signedPercent,
	"perWeek":  formatPerWeek,
}).Parse(emailHTML))

// emailData is the template's view of a report, plus the few strings the report
// itself does not carry.
type emailData struct {
	Report      Report
	DisplayName string
	Lifts       []LiftSlice
	MaxLift     float64
	Weekdays    []float64
	MaxWeekday  float64
	BestWeekday int
}

// Subject is the recap's subject line, naming the lifter because every recap
// goes to the same address — without the name, twelve monthly emails would be
// indistinguishable in an inbox.
func Subject(displayName string, rep Report) string {
	return fmt.Sprintf("Racked: %s — %s", displayName, rep.Period.Label)
}

// RenderEmail produces the recap's HTML body.
func RenderEmail(displayName string, rep Report) (string, error) {
	data := emailData{
		Report:      rep,
		DisplayName: displayName,
		Lifts:       rep.Lifts,
		Weekdays:    rep.Weekdays,
		BestWeekday: rep.BestWeekday,
	}
	// Only the top few lifts: an email is skimmed once, and a lift that carried
	// two percent of the month is not what the reader opened it for.
	if len(data.Lifts) > 5 {
		data.Lifts = data.Lifts[:5]
	}
	for _, l := range data.Lifts {
		data.MaxLift = math.Max(data.MaxLift, l.VolumeLb)
	}
	for _, v := range data.Weekdays {
		data.MaxWeekday = math.Max(data.MaxWeekday, v)
	}

	var buf bytes.Buffer
	if err := emailTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func barPct(value, max float64) string {
	if max <= 0 || value <= 0 {
		return "0"
	}
	return fmt.Sprintf("%.1f", math.Min(value/max, 1)*barWidth)
}

func formatEmailPercent(fraction float64) string {
	if math.IsNaN(fraction) || math.IsInf(fraction, 0) {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", fraction*100)
}

// formatEmailDuration reads as a person would say it: "48m", "1h 12m".
func formatEmailDuration(d time.Duration) string {
	total := int(math.Round(d.Minutes()))
	if total <= 0 {
		return "—"
	}
	if total < 60 {
		return fmt.Sprintf("%dm", total)
	}
	if total%60 == 0 {
		return fmt.Sprintf("%dh", total/60)
	}
	return fmt.Sprintf("%dh %dm", total/60, total%60)
}

func formatEmailDate(t time.Time) string {
	return t.Format("January 2")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func weekdayName(i int) string {
	if i < 0 || i > 6 {
		return ""
	}
	return time.Weekday(i).String()
}

// formatPerWeek renders a training frequency to one decimal, because the whole
// number loses the difference between twice a week and nearly three times.
func formatPerWeek(perWeek float64) string {
	if math.IsNaN(perWeek) || math.IsInf(perWeek, 0) || perWeek <= 0 {
		return "0"
	}
	return fmt.Sprintf("%.1f", perWeek)
}

// signed renders a delta the way the email quotes it.
func signedPercent(fraction float64) string {
	pct := int(math.Round(fraction * 100))
	if pct > 0 {
		return fmt.Sprintf("+%d%%", pct)
	}
	return fmt.Sprintf("%d%%", pct)
}

// formatWeightLb renders a scale reading, to one decimal and without the
// trailing ".0" on a whole number.
//
// Separate from formatLb, which rounds. That is right for tonnage — nobody reads
// a half-pound of a five-figure total — and wrong here, where the half-pound is
// most of what changed: a month of careful eating rounds to no change at all.
func formatWeightLb(lb float64) string {
	if math.IsNaN(lb) || math.IsInf(lb, 0) || lb <= 0 {
		return "0"
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", lb), ".0")
}

// signedWeightLb renders a bodyweight delta with its sign, so a loss and a gain
// are told apart at a glance rather than by comparing two numbers.
func signedWeightLb(lb float64) string {
	if math.IsNaN(lb) || math.IsInf(lb, 0) {
		return "0"
	}
	body := strings.TrimSuffix(fmt.Sprintf("%.1f", math.Abs(lb)), ".0")
	switch {
	case lb > 0:
		return "+" + body
	case lb < 0:
		// A hyphen-minus, not the typographic minus the page uses: mail clients
		// that fall back through a font stack render U+2212 as a box often
		// enough that the character is not worth the alignment it buys.
		return "-" + body
	default:
		return "0"
	}
}

// trimmed keeps the template readable in source without shipping the leading
// indentation into every mail client.
var emailHTML = strings.TrimSpace(`
<!DOCTYPE html>
<html lang="en"><head><meta charset="UTF-8"><title>Racked</title></head>
<body style="margin:0;padding:24px 16px;background:#f1f5f9;font-family:-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,sans-serif;">
<div style="max-width:600px;margin:0 auto;background:#fff;border-radius:10px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,.08);">

  <div style="padding:28px 30px;background:#2a1250;color:#eae0ff;">
    <div style="font-size:12px;font-weight:700;letter-spacing:.18em;text-transform:uppercase;opacity:.75;">Racked · {{ .Report.Period.Label }}</div>
    <div style="font-size:15px;margin-top:10px;opacity:.85;">{{ .DisplayName }} lifted</div>
    <div style="font-size:44px;font-weight:800;line-height:1.1;color:#fff;">{{ lb .Report.Totals.VolumeLb }} lb</div>
    {{ with .Report.Comparison }}{{ if gt .Count 0 }}
      <div style="font-size:15px;margin-top:6px;opacity:.85;">That's {{ .Count }} {{ .Label }}.</div>
    {{ end }}{{ end }}
    {{ with .Report.Change }}{{ if .VolumePct }}
      <div style="font-size:13px;margin-top:8px;opacity:.7;">{{ signed .VolumePct }} on the period before</div>
    {{ end }}{{ end }}
  </div>

  {{ with .Report.Archetype }}{{ if .Name }}
  <div style="padding:20px 30px;background:#f8fafc;border-bottom:1px solid #e2e8f0;text-align:center;">
    <div style="font-size:12px;font-weight:700;letter-spacing:.12em;text-transform:uppercase;color:#94a3b8;">Your lifter type</div>
    <div style="font-size:22px;font-weight:800;color:#1e293b;margin-top:4px;">{{ .Name }}</div>
    <div style="font-size:14px;color:#64748b;margin-top:2px;">{{ .Description }}</div>
  </div>
  {{ end }}{{ end }}

  <div style="padding:22px 30px;">
    <table style="width:100%;border-collapse:collapse;text-align:center;margin-bottom:22px;">
      <tr>
        <td style="padding:6px;"><div style="font-size:24px;font-weight:800;color:#1e293b;">{{ .Report.Totals.Sessions }}</div><div style="font-size:11px;letter-spacing:.1em;text-transform:uppercase;color:#94a3b8;">sessions</div></td>
        <td style="padding:6px;"><div style="font-size:24px;font-weight:800;color:#1e293b;">{{ .Report.Totals.Sets }}</div><div style="font-size:11px;letter-spacing:.1em;text-transform:uppercase;color:#94a3b8;">sets</div></td>
        <td style="padding:6px;"><div style="font-size:24px;font-weight:800;color:#1e293b;">{{ .Report.Totals.Reps }}</div><div style="font-size:11px;letter-spacing:.1em;text-transform:uppercase;color:#94a3b8;">reps</div></td>
        <td style="padding:6px;"><div style="font-size:24px;font-weight:800;color:#1e293b;">{{ .Report.Streak.LongestWeeks }}</div><div style="font-size:11px;letter-spacing:.1em;text-transform:uppercase;color:#94a3b8;">week streak</div></td>
      </tr>
    </table>

    {{ if or .Report.MostImproved .Report.HeaviestSet .Report.FastestSession }}
    <div style="font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#94a3b8;margin-bottom:10px;padding-bottom:5px;border-bottom:1px solid #f1f5f9;">Moments</div>
    <table style="width:100%;border-collapse:collapse;font-size:14px;margin-bottom:24px;">
      {{ with .Report.MostImproved }}
      <tr><td style="padding:7px 0;color:#64748b;">Most improved</td><td style="padding:7px 0;text-align:right;color:#1e293b;"><strong>{{ .ExerciseName }}</strong> {{ signed .GainPct }}</td></tr>
      {{ end }}
      {{ with .Report.HeaviestSet }}
      <tr><td style="padding:7px 0;color:#64748b;">Heaviest set</td><td style="padding:7px 0;text-align:right;color:#1e293b;"><strong>{{ lb .WeightLb }} lb × {{ .Reps }}</strong> {{ .ExerciseName }}, {{ date .PerformedOn }}</td></tr>
      {{ end }}
      {{ with .Report.FastestSession }}
      <tr><td style="padding:7px 0;color:#64748b;">Fastest session</td><td style="padding:7px 0;text-align:right;color:#1e293b;"><strong>{{ dur .Duration }}</strong> {{ .ProgramDayName }}, {{ date .PerformedOn }}</td></tr>
      {{ end }}
    </table>
    {{ end }}

    {{ with .Report.Bodyweight }}
    <div style="font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#94a3b8;margin-bottom:10px;padding-bottom:5px;border-bottom:1px solid #f1f5f9;">Bodyweight</div>
    <table style="width:100%;border-collapse:collapse;font-size:14px;margin-bottom:24px;">
      {{ if .ChangeLb }}
      <tr>
        <td style="padding:7px 0;color:#64748b;">Start to end</td>
        <td style="padding:7px 0;text-align:right;color:#1e293b;">
          <strong>{{ weight .StartLb }} &rarr; {{ weight .EndLb }} lb</strong>
          {{ signedLb .ChangeLb }} lb{{ if .ChangePct }} &middot; {{ signed .ChangePct }}{{ end }}
        </td>
      </tr>
      <tr>
        <td style="padding:7px 0;color:#64748b;">Range</td>
        <td style="padding:7px 0;text-align:right;color:#1e293b;">
          {{ weight .LowLb }}&ndash;{{ weight .HighLb }} lb over {{ len .Points }} weigh-ins
        </td>
      </tr>
      {{ else }}
      <!-- One weigh-in is a reading, not a series: there is a number to report
           and no change to quote, so the row says the number and stops. -->
      <tr>
        <td style="padding:7px 0;color:#64748b;">Weighed in at</td>
        <td style="padding:7px 0;text-align:right;color:#1e293b;"><strong>{{ weight .EndLb }} lb</strong></td>
      </tr>
      {{ end }}
    </table>
    {{ end }}

    {{ if .Lifts }}
    <div style="font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#94a3b8;margin-bottom:10px;padding-bottom:5px;border-bottom:1px solid #f1f5f9;">Where the weight went</div>
    {{ with .Report.Split }}{{ if gt .Assistance.VolumeLb 0.0 }}
    <!-- Only where there is assistance to account for. A lifter who does none
         should not read a line telling them 100% of their work was the program's
         — that is not news, it is the default. -->
    <div style="font-size:12px;color:#64748b;margin:-4px 0 10px;">
      Main lifts {{ pct .Main.Share }} &middot; assistance {{ pct .Assistance.Share }}
      across {{ .Assistance.Lifts }} {{ plural .Assistance.Lifts "movement" "movements" }}
    </div>
    {{ end }}{{ end }}
    <table style="width:100%;border-collapse:collapse;font-size:13px;margin-bottom:24px;">
      {{ range .Lifts }}
      <tr>
        <td style="padding:4px 8px 4px 0;color:#1e293b;white-space:nowrap;">
          {{ .ExerciseName }}{{ if .IsAssistance }}<span style="color:#94a3b8;"> &middot; assistance</span>{{ end }}
        </td>
        <td style="padding:4px 0;width:100%;">
          <div style="background:#7b2ff7;height:10px;border-radius:5px;width:{{ barPct .VolumeLb $.MaxLift }}%;"></div>
        </td>
        <td style="padding:4px 0 4px 8px;color:#64748b;text-align:right;white-space:nowrap;">{{ lb .VolumeLb }} lb</td>
      </tr>
      {{ end }}
    </table>
    {{ end }}

    {{ if ge .BestWeekday 0 }}
    <div style="font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#94a3b8;margin-bottom:10px;padding-bottom:5px;border-bottom:1px solid #f1f5f9;">Most productive day · {{ weekday .BestWeekday }}</div>
    <table style="width:100%;border-collapse:collapse;font-size:13px;margin-bottom:24px;">
      {{ range $i, $v := .Weekdays }}
      <tr>
        <td style="padding:3px 8px 3px 0;color:#64748b;white-space:nowrap;">{{ weekday $i }}</td>
        <td style="padding:3px 0;width:100%;">
          <div style="background:{{ if eq $i $.BestWeekday }}#7b2ff7{{ else }}#cbd5e1{{ end }};height:8px;border-radius:4px;width:{{ barPct $v $.MaxWeekday }}%;"></div>
        </td>
        <td style="padding:3px 0 3px 8px;color:#94a3b8;text-align:right;white-space:nowrap;">{{ lb $v }}</td>
      </tr>
      {{ end }}
    </table>
    {{ end }}

    {{ if .Report.PRs }}
    <div style="font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#94a3b8;margin-bottom:10px;padding-bottom:5px;border-bottom:1px solid #f1f5f9;">{{ len .Report.PRs }} personal {{ plural (len .Report.PRs) "record" "records" }}</div>
    <table style="width:100%;border-collapse:collapse;font-size:14px;margin-bottom:24px;">
      {{ range .Report.PRs }}
      <tr><td style="padding:5px 0;color:#1e293b;">{{ .ExerciseName }}</td><td style="padding:5px 0;text-align:right;color:#64748b;">{{ lb .WeightLb }} lb × {{ .Reps }}{{ if eq (printf "%s" .Kind) "e1rm" }} · est. max{{ end }}</td></tr>
      {{ end }}
    </table>
    {{ end }}

    {{ if .Report.Milestones }}
    <div style="font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#94a3b8;margin-bottom:10px;padding-bottom:5px;border-bottom:1px solid #f1f5f9;">Milestones</div>
    <table style="width:100%;border-collapse:collapse;font-size:14px;margin-bottom:24px;">
      {{ range .Report.Milestones }}
      <tr><td style="padding:5px 0;color:#1e293b;">{{ .Label }}</td><td style="padding:5px 0;text-align:right;color:#64748b;">{{ date .PerformedOn }}</td></tr>
      {{ end }}
    </table>
    {{ end }}

    {{ if .Report.Deloads }}
    <div style="font-size:12px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:#94a3b8;margin-bottom:10px;padding-bottom:5px;border-bottom:1px solid #f1f5f9;">Stalls and comebacks</div>
    <table style="width:100%;border-collapse:collapse;font-size:14px;">
      {{ range .Report.Deloads }}
      <tr><td style="padding:5px 0;color:#1e293b;">{{ .ExerciseName }} {{ lb .FromLb }} → {{ lb .ToLb }} lb</td><td style="padding:5px 0;text-align:right;color:#64748b;">{{ if .Recovered }}won it back{{ else }}still climbing{{ end }}</td></tr>
      {{ end }}
    </table>
    {{ end }}
  </div>

  <div style="padding:14px 30px;background:#f8fafc;border-top:1px solid #e2e8f0;font-size:12px;color:#94a3b8;text-align:center;">
    🏋️ Iron Temple ·
    {{- if eq (printf "%s" .Report.Attendance.Basis) "weekday" }}
      {{ pct .Report.Attendance.Rate }} of your scheduled sessions
    {{- else }}
      {{ perWeek .Report.Attendance.SessionsPerWeek }} sessions a week
    {{- end }}
  </div>
</div>
</body></html>
`)
