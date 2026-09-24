package api

import (
	"context"
	"net/http"
	"sort"
	"time"

	"gitea.homelab/gitadmin/iron-temple/api/internal/racked"
)

// How the lifters on this install compare over a week, month or year.
//
// # NO NEW STATISTICS
//
// Every number here is read off racked.Report, which is the same report each
// lifter sees on their own Racked page, computed by the same code over the same
// rows. Nothing is recomputed and nothing is invented. That is the property the
// whole social layer rests on — the install must not hold two answers about one
// history — and a leaderboard is where it would be most tempting to break it,
// because "just sum the volume" looks so much cheaper than running the report.
//
// # ONE REQUEST, EVERY BOARD
//
// There is no `metric` parameter. One pass of buildRacked per lifter already
// computes every figure below, so a per-metric endpoint would re-run the heaviest
// query in the app to read a different field off the same result. The client gets
// all the boards and switches between them without a request.
//
// # WHY THE ORDER OF THE BOARDS IS A DECISION
//
// Boards are returned in the order a surface should offer them, and the first is
// the one to open on. That ordering is an opinion about fairness, not a
// convenience:
//
// Raw tonnage ranks people by how much they weigh and how long they have been
// lifting. On an install shared by a household that is a game the newest lifter
// cannot win and the strongest cannot lose, and putting it first would tell
// somebody six weeks in that they are losing at training. So volume is last —
// available, because people want it, and not the thing the page opens on.
//
// What leads instead is how often you trained and how well you kept your own
// schedule: measures relative to the lifter rather than to the others.
//
// # COST
//
// N lifters × buildRacked, which is five queries each. Bounded by the account
// count, which the install's owner sets by hand — this is not an endpoint that can
// be made expensive by a stranger.
//
// Run SEQUENTIALLY, deliberately. The deployment has one small connection pool
// shared with the recap reporter and the session sweeper (see NewServer), and
// firing a lifter's worth of queries per goroutine is how a leaderboard starves
// the request that is actually logging a set.

// Board metric identifiers. Strings on the wire so a client can label and format
// them; ordering is the slice below, not this list.
const (
	boardSessionsPerWeek = "sessionsPerWeek"
	boardAttendance      = "attendance"
	boardImprovement     = "improvement"
	boardStreak          = "streak"
	boardVolume          = "volume"
)

// Units tell a client how to render a value without parsing the metric name.
const (
	unitPerWeek = "per_week"
	unitPercent = "percent"
	unitCount   = "count"
	unitPounds  = "pounds"
)

// lifterMetrics is one lifter's figures, lifted off their report.
//
// A middle step rather than building the boards directly, because the boards are
// columns and the reports arrive as rows: every lifter is read once, then each
// board sorts the same slice a different way.
type lifterMetrics struct {
	lifter lifterDTO

	sessionsPerWeek float64
	volumeLb        float64
	streakWeeks     float64

	// attendanceRate is only meaningful when the lifter's program carries
	// scheduled weekdays. racked's own comment is explicit that Rate must not be
	// shown otherwise — it is zero against a denominator nobody entered — so this
	// is nil rather than 0 for them, and they are left off that board entirely.
	attendanceRate *float64

	// improvement is nil for a lifter who has not performed any one lift twice in
	// the period: there is no improvement to report, which is different from an
	// improvement of nothing.
	improvementPct  *float64
	improvementLift string
}

// getLeaderboard serves every board for one period.
func (s *Server) getLeaderboard(w http.ResponseWriter, r *http.Request) {
	kind, on, today, ok := s.rackedWindow(w, r)
	if !ok {
		return
	}

	boards, period, err := s.computeBoards(r.Context(), kind, on, today)
	if err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, leaderboardDTO{
		Period: rackedPeriodToDTO(period),
		Boards: boards,
	})
}

// computeBoards ranks every lifter on the install over one window.
//
// Extracted from getLeaderboard for rackedWindow's reason, and it matters more
// here than there. The crown reconciler (achievements.go) has to agree with this
// page about who is leading, and the file header's rule is that the install must
// not hold two answers about one history — a second implementation of "who is
// top" would be exactly that, and it would show up as a crown beside a name that
// the leaderboard says belongs to somebody else.
//
// The cost described in the file header is this function's. It is called once per
// request to /leaderboard and once per hourly reconcile, and nowhere else.
func (s *Server) computeBoards(
	ctx context.Context, kind racked.PeriodKind, on, today time.Time,
) ([]leaderboardBoardDTO, racked.Period, error) {
	var period racked.Period

	lifters, err := s.q.ListLifters(ctx)
	if err != nil {
		return nil, period, err
	}

	metrics := make([]lifterMetrics, 0, len(lifters))
	for _, row := range lifters {
		report, err := s.buildRacked(ctx, row.ID, kind, on, today)
		if err != nil {
			return nil, period, err
		}
		// Every report covers the same window, so the last one read is as good as
		// the first. Taken from a report rather than recomputed here so that the
		// period a client labels the page with is the period the figures were
		// actually measured over.
		period = report.Period

		m := lifterMetrics{
			lifter: lifterDTO{
				ID:          row.ID,
				Username:    row.Username,
				DisplayName: row.DisplayName,
				AvatarColor: row.AvatarColor,
				HasAvatar:   row.AvatarEtag != "",
				AvatarEtag:  row.AvatarEtag,
			},
			sessionsPerWeek: report.Attendance.SessionsPerWeek,
			volumeLb:        report.Totals.VolumeLb,
			streakWeeks:     float64(report.Streak.CurrentWeeks),
		}
		if report.Attendance.Basis == racked.AttendanceWeekday {
			rate := report.Attendance.Rate
			m.attendanceRate = &rate
		}
		if report.MostImproved != nil {
			pct := report.MostImproved.GainPct
			m.improvementPct = &pct
			m.improvementLift = report.MostImproved.ExerciseName
		}
		metrics = append(metrics, m)
	}

	return buildBoards(metrics), period, nil
}

// buildBoards turns one row per lifter into one board per metric.
//
// The slice order is the answer to "which board should the page open on" — see
// the header comment. Volume is last on purpose.
func buildBoards(metrics []lifterMetrics) []leaderboardBoardDTO {
	return []leaderboardBoardDTO{
		board(boardSessionsPerWeek, "Sessions a week", unitPerWeek,
			"How often each lifter trained, over the whole period rather than "+
				"over the weeks they showed up.",
			metrics, func(m lifterMetrics) (float64, string, bool) {
				return m.sessionsPerWeek, "", true
			}),
		board(boardAttendance, "Attendance", unitPercent,
			"Sessions performed against the days each lifter's own program asked "+
				"for. Lifters whose program carries no weekdays have no schedule to "+
				"be measured against and are not listed.",
			metrics, func(m lifterMetrics) (float64, string, bool) {
				if m.attendanceRate == nil {
					return 0, "", false
				}
				return *m.attendanceRate, "", true
			}),
		board(boardImprovement, "Most improved", unitPercent,
			"Each lifter's best gain on a single lift, against their own earlier "+
				"weight in the same period.",
			metrics, func(m lifterMetrics) (float64, string, bool) {
				if m.improvementPct == nil {
					return 0, "", false
				}
				return *m.improvementPct, m.improvementLift, true
			}),
		board(boardStreak, "Week streak", unitCount,
			"Consecutive weeks trained, ending at the last week each lifter "+
				"trained within the period.",
			metrics, func(m lifterMetrics) (float64, string, bool) {
				return m.streakWeeks, "", true
			}),
		board(boardVolume, "Volume", unitPounds,
			"Total weight moved. Worth knowing and a poor contest: it ranks "+
				"lifters by bodyweight and training age as much as by effort, which "+
				"is why it is not the board this page opens on.",
			metrics, func(m lifterMetrics) (float64, string, bool) {
				return m.volumeLb, "", true
			}),
	}
}

// board ranks one metric across the lifters.
//
// read returns the value, an optional detail, and whether the metric is DEFINED
// for that lifter. Defined is not the same as non-zero: a lifter who trained
// nothing has a volume of zero and belongs on the board saying so, where a lifter
// with no schedule has no attendance at all and does not. Hiding the former would
// flatter the board and make attendance meaningless.
func board(
	metric, label, unit, note string,
	metrics []lifterMetrics,
	read func(lifterMetrics) (float64, string, bool),
) leaderboardBoardDTO {
	type scored struct {
		m      lifterMetrics
		value  float64
		detail string
	}

	rows := make([]scored, 0, len(metrics))
	for _, m := range metrics {
		value, detail, defined := read(m)
		if !defined {
			continue
		}
		rows = append(rows, scored{m: m, value: value, detail: detail})
	}

	// Highest first, with the lifter id breaking ties. The tiebreak is not
	// cosmetic: without it two lifters on the same figure would swap places
	// between requests, and a leaderboard that reorders itself on refresh reads as
	// one that cannot be trusted.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].value != rows[j].value {
			return rows[i].value > rows[j].value
		}
		return rows[i].m.lifter.ID < rows[j].m.lifter.ID
	})

	entries := make([]leaderboardEntryDTO, 0, len(rows))
	for i, row := range rows {
		// Equal figures share a rank, and the next distinct figure skips the ones
		// they used up: 1, 2, 2, 4. Computed here rather than left to the client,
		// which would otherwise have to rediscover tie semantics per surface — and
		// a board that showed two lifters on identical numbers as 2nd and 3rd
		// would be inventing a difference the data does not hold.
		rank := i + 1
		if i > 0 && row.value == rows[i-1].value {
			rank = int(entries[i-1].Rank)
		}
		entries = append(entries, leaderboardEntryDTO{
			Rank:   int32(rank),
			Lifter: row.m.lifter,
			Value:  row.value,
			Detail: row.detail,
		})
	}

	return leaderboardBoardDTO{
		Metric:  metric,
		Label:   label,
		Unit:    unit,
		Note:    note,
		Entries: entries,
	}
}
