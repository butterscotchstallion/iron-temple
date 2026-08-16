package racked

import (
	"math"
	"strconv"
	"strings"
)

// Comparison restates a tonnage as a count of something a person can picture.
// It is the most quotable line in the recap and the reason the headline number
// lands at all: 84,000 lb means nothing until it is six grand pianos.
type Comparison struct {
	Count  int
	Label  string // already pluralised for Count
	UnitLb float64
}

// comparisonUnit is one object the volume can be counted in.
type comparisonUnit struct {
	singular string
	plural   string
	lb       float64
}

// comparisonUnits run lightest to heaviest. compare picks the heaviest unit the
// lifter cleared at least one of, so the count stays small and the object stays
// impressive — 6 grand pianos rather than 1,866 forty-five pound plates.
//
// Weights are round approximations on purpose. Nobody is checking the mass of a
// school bus, and a figure precise to the pound would imply they should.
var comparisonUnits = []comparisonUnit{
	{"45 lb plate", "45 lb plates", 45},
	{"grizzly bear", "grizzly bears", 600},
	{"grand piano", "grand pianos", 800},
	{"pickup truck", "pickup trucks", 5_000},
	{"school bus", "school buses", 24_000},
	{"Boeing 737", "Boeing 737s", 90_000},
	{"blue whale", "blue whales", 300_000},
	{"Statue of Liberty", "Statues of Liberty", 450_000},
	{"Eiffel Tower", "Eiffel Towers", 22_000_000},
}

// compare converts a volume into the largest whole count it supports. A volume
// too small for even the lightest unit yields a zero Comparison, which the
// surfaces read as "nothing to say yet" rather than rendering "0 plates".
func compare(volumeLb float64) Comparison {
	var out Comparison
	for _, u := range comparisonUnits {
		n := int(math.Floor(volumeLb / u.lb))
		if n < 1 {
			break
		}
		label := u.plural
		if n == 1 {
			label = u.singular
		}
		out = Comparison{Count: n, Label: label, UnitLb: u.lb}
	}
	return out
}

// formatLb renders a weight the way the recap quotes it: whole pounds, grouped
// in threes. It matches formatVolume in the UI so a milestone label built here
// reads the same as a number rendered there.
func formatLb(lb float64) string {
	if math.IsNaN(lb) || math.IsInf(lb, 0) || lb <= 0 {
		return "0"
	}
	digits := strconv.FormatInt(int64(math.Round(lb)), 10)

	var b strings.Builder
	lead := len(digits) % 3
	if lead > 0 {
		b.WriteString(digits[:lead])
	}
	for i := lead; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}
