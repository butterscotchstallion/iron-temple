package racked

import (
	"math"
	"time"
)

// Archetype is the recap's opening line: a name for how the lifter trained,
// derived from pace, frequency and when they showed up. It is the one figure
// here that is not a measurement, and it is deliberately generous — a recap
// that opens by calling someone inconsistent is a recap nobody opens twice.
type Archetype struct {
	Name        string
	Description string
}

// Archetype thresholds. Named rather than inlined because they are the whole of
// the classification: reading these five numbers should tell you what the
// labels mean without following the branches below.
const (
	// machineSessionsPerWeek is a frequency past what any of the seeded
	// programs prescribe — four or more sessions a week is showing up extra.
	machineSessionsPerWeek = 4.0
	// grinderMinutes is a session long enough that the lifter is clearly
	// working through rest timers rather than racing them.
	grinderMinutes = 75.0
	// sprinterMinutes is a session short enough to be deliberate about it.
	sprinterMinutes = 35.0
	// weekendShare is the fraction of sessions on a Saturday or Sunday that
	// makes the weekend the point rather than a coincidence.
	weekendShare = 0.6
	// minSessionsForPace is how many timed sessions the average needs before
	// pace is allowed to decide anything.
	minSessionsForPace = 3
)

// archetype classifies the period. Order matters: the first rule that matches
// wins, running from the most remarkable trait to the least, so that a lifter
// who trains five times a week is a Machine before anything else is considered.
func archetype(sessions []session, start, end time.Time) Archetype {
	if len(sessions) == 0 {
		return Archetype{}
	}

	perWeek := float64(len(sessions)) / weeksBetween(start, end)
	avgMinutes, timed := averageMinutes(sessions)
	weekends := 0
	for _, s := range sessions {
		if wd := s.PerformedOn.Weekday(); wd == time.Saturday || wd == time.Sunday {
			weekends++
		}
	}
	weekendFraction := float64(weekends) / float64(len(sessions))
	pacey := timed >= minSessionsForPace

	switch {
	case perWeek >= machineSessionsPerWeek:
		return Archetype{
			Name:        "The Machine",
			Description: "You trained more often than the program even asked for.",
		}
	case pacey && avgMinutes >= grinderMinutes:
		return Archetype{
			Name:        "The Grinder",
			Description: "Long sessions, no rush. You stayed until the work was done.",
		}
	case pacey && avgMinutes <= sprinterMinutes:
		return Archetype{
			Name:        "The Sprinter",
			Description: "In, under the bar, and out. Nobody moves through a session faster.",
		}
	case weekendFraction >= weekendShare:
		return Archetype{
			Name:        "The Weekend Warrior",
			Description: "The week belongs to everyone else. Saturday belongs to the bar.",
		}
	default:
		return Archetype{
			Name:        "The Metronome",
			Description: "Same days, same work, week after week. Progress is made of this.",
		}
	}
}

// weeksBetween is the period's length in weeks, never less than one so that a
// single session in a short window cannot divide its way into a huge frequency.
func weeksBetween(start, end time.Time) float64 {
	days := end.Sub(start).Hours()/24 + 1
	return math.Max(1, days/7)
}

// averageMinutes averages the sessions that have a trustworthy duration, and
// reports how many there were. A period of sessions nobody tapped Finish on
// yields no average, and the caller must not read pace into that silence.
func averageMinutes(sessions []session) (float64, int) {
	var total time.Duration
	var n int
	for _, s := range sessions {
		if d := s.Duration(); d > 0 {
			total += d
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return total.Minutes() / float64(n), n
}
