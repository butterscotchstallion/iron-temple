package api

// Response DTOs. These mirror the OpenAPI schemas exactly (camelCase JSON, plain
// scalar types), keeping the generated sqlc row structs — with their snake_case
// tags and pgtype columns — out of the wire contract.

type errorDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type healthDTO struct {
	Status      string `json:"status"`
	Version     string `json:"version,omitempty"`
	Environment string `json:"environment,omitempty"`
}

// userDTO is the profile shape. It has no password field of any kind, so a
// hash cannot reach the wire by a careless edit — the type simply has nowhere
// to put one.
type userDTO struct {
	ID          int32  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	// AvatarColor is a hex colour for the initials chip, or "" to let the UI
	// derive one from the id.
	AvatarColor string `json:"avatarColor"`
	IsAdmin     bool   `json:"isAdmin"`
	// CurrentProgramID is the program the user last opened, so the app can land
	// them on it. Omitted until they have opened one, which the UI reads as
	// "fall back to the program of my most recent session".
	CurrentProgramID *int32 `json:"currentProgramId,omitempty"`
	// HasAvatar tells the UI whether to render an <img> or the initials chip,
	// so it needn't request an image that may 404.
	HasAvatar bool `json:"hasAvatar"`
	// AvatarEtag is appended to the avatar URL as a cache-buster, so a new
	// upload appears immediately instead of after the cache expires.
	AvatarEtag string `json:"avatarEtag,omitempty"`
}

type registrationStatusDTO struct {
	Open bool `json:"open"`
}

type avatarDTO struct {
	Etag string `json:"etag"`
}

// Which list an exercise in a session came from: the program's own prescription,
// or the lifter's assistance. Derived at read time from whether the exercise has
// a program_day_exercises row for the day, never stored on the set.
const (
	exerciseKindMain       = "main"
	exerciseKindAssistance = "assistance"
)

// progressionFixed is the status reported for assistance work. It is not one of
// progression.Status: the engine never produces it, because no engine runs on
// assistance at all. It says "this weight was carried forward, not computed",
// which is what stops the UI reaching for a stall badge that has no meaning here.
const progressionFixed = "fixed"

type exerciseDTO struct {
	ID          int32  `json:"id"`
	Name        string `json:"name"`
	MuscleGroup string `json:"muscleGroup"`
	Equipment   string `json:"equipment"`
	IsAccessory bool   `json:"isAccessory"`
	// IsCustom marks a movement this lifter created, which is the only kind
	// anyone may delete.
	IsCustom bool `json:"isCustom"`
}

type exerciseHistoryPointDTO struct {
	PerformedOn string  `json:"performedOn"`
	WeightLb    float64 `json:"weightLb"`
	Reps        int32   `json:"reps"`
	Completed   bool    `json:"completed"`
}

type programSummaryDTO struct {
	ID              int32  `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	ProgressionKind string `json:"progressionKind"`
}

// programDTO embeds the summary so its fields marshal inline (OpenAPI allOf).
type programDTO struct {
	programSummaryDTO
	Days []programDayDTO `json:"days"`
}

type programDayDTO struct {
	ID       int32  `json:"id"`
	Name     string `json:"name"`
	Position int32  `json:"position"`
	Weekday  *int32 `json:"weekday"`
	// Exercises is the program's own prescription: shared by every account and
	// never edited. Assistance is the calling lifter's overlay on top of it.
	Exercises  []programDayExerciseDTO   `json:"exercises"`
	Assistance []programDayAssistanceDTO `json:"assistance"`
}

// programDayAssistanceDTO has no starting weight and no progression, which is
// the difference from programDayExerciseDTO in one line: the engine drives the
// prescription, and assistance is driven by what the lifter last did.
type programDayAssistanceDTO struct {
	ID           int32  `json:"id"`
	ExerciseID   int32  `json:"exerciseId"`
	ExerciseName string `json:"exerciseName"`
	Position     int32  `json:"position"`
	Sets         int32  `json:"sets"`
	Reps         int32  `json:"reps"`
	// WeightLb is the fallback used until the lift has been logged once; after
	// that the prescription carries forward from the last performance.
	WeightLb float64 `json:"weightLb"`
}

type programDayExerciseDTO struct {
	ID               int32   `json:"id"`
	ExerciseID       int32   `json:"exerciseId"`
	ExerciseName     string  `json:"exerciseName"`
	Position         int32   `json:"position"`
	Sets             int32   `json:"sets"`
	Reps             int32   `json:"reps"`
	StartingWeightLb float64 `json:"startingWeightLb"`
	RestSeconds      int32   `json:"restSeconds"`
}

type prescribedSessionDTO struct {
	ProgramID      int32                   `json:"programId"`
	ProgramDayID   int32                   `json:"programDayId"`
	ProgramDayName string                  `json:"programDayName"`
	Exercises      []prescribedExerciseDTO `json:"exercises"`
	// Layoff is nil unless the lifter has been away a week or more, so its mere
	// presence is the client's cue to ask whether to ease back in. Reported
	// whether or not the deload was applied — the weights above already reflect
	// the answer, and this says what the question was.
	Layoff *layoffDTO `json:"layoff"`
}

// layoffDTO describes time away from training and what easing back in would
// cost, so the UI can ask a specific question ("it's been 3 weeks — take 30%
// off?") instead of a vague one.
type layoffDTO struct {
	Weeks         int    `json:"weeks"`
	LastTrainedOn string `json:"lastTrainedOn"`
	// DeloadPct is a fraction (0.30 is 30%), matching rackedChangeDTO's
	// convention for percentages on this wire.
	DeloadPct float64 `json:"deloadPct"`
	// Applied is whether the weights in this response actually have the cut in
	// them. False is the default: a layoff deload is offered, never imposed.
	Applied bool `json:"applied"`
}

type prescribedExerciseDTO struct {
	ExerciseID   int32              `json:"exerciseId"`
	ExerciseName string             `json:"exerciseName"`
	Kind         string             `json:"kind"`
	Sets         int32              `json:"sets"`
	Reps         int32              `json:"reps"`
	WeightLb     float64            `json:"weightLb"`
	RestSeconds  int32              `json:"restSeconds"`
	Progression  progressionInfoDTO `json:"progression"`
}

// progressionInfoDTO explains why the engine chose a lift's weight, so the UI
// can surface an impending stall or a deload instead of a bare number.
type progressionInfoDTO struct {
	// Status is one of the progression.Status values
	// (start|advance|hold|deload|layoff), or "fixed" for assistance.
	Status string `json:"status"`
	// FailureCount is the consecutive trailing failures at the working weight.
	//
	// int, not int32, to match progression.Plan.FailureCount — these two and
	// FailuresBeforeDeload are engine-domain counts, not database SERIAL ids like
	// the int32 fields above. Converting to int32 here bought nothing (openapi.yaml
	// declares plain `type: integer`, no int32 format, and the wire bytes are
	// identical) while costing a gosec G115 int -> int32 overflow finding.
	FailureCount int `json:"failureCount"`
	// FailuresBeforeDeload is the threshold at which a stall triggers a deload.
	FailuresBeforeDeload int `json:"failuresBeforeDeload"`
	// PreviousWeightLb is the weight just worked (advanced past, repeated, or
	// deloaded from); 0 when there is no history.
	PreviousWeightLb float64 `json:"previousWeightLb"`
	// LayoffPct is the fraction taken off this lift for time away from
	// training (0.30 is 30%), 0 when none was. Per-lift as well as on the
	// session, because a layoff does not necessarily reach every lift: one
	// already deloaded further for a stall keeps its own, deeper cut.
	LayoffPct float64 `json:"layoffPct"`
}

type sessionSetDTO struct {
	ID           int32   `json:"id"`
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
	Kind         string  `json:"kind"`
	SetNumber    int32   `json:"setNumber"`
	TargetReps   int32   `json:"targetReps"`
	ActualReps   *int32  `json:"actualReps"`
	WeightLb     float64 `json:"weightLb"`
	Completed    bool    `json:"completed"`
	RestSeconds  int32   `json:"restSeconds"`
}

type sessionDTO struct {
	ID             int32  `json:"id"`
	ProgramID      int32  `json:"programId"`
	ProgramName    string `json:"programName"`
	ProgramDayID   int32  `json:"programDayId"`
	ProgramDayName string `json:"programDayName"`
	PerformedOn    string `json:"performedOn"`
	Notes          string `json:"notes"`
	CreatedAt      string `json:"createdAt"`
	// FinishedAt is nil until the session is finished by hand; a session can be
	// over (IsOver) without one, having simply aged out.
	FinishedAt *string `json:"finishedAt"`
	IsOver     bool    `json:"isOver"`
	// BodyweightLb is nil when the lifter did not weigh in, which is a different
	// answer from any number: the weight-loss series is the days that were
	// actually measured.
	BodyweightLb *float64 `json:"bodyweightLb"`
	// LastWeighIn is the most recent weigh-in from another session, which is what
	// lets the session screen open its box pre-filled. Nil until there is one.
	LastWeighIn *weighInDTO     `json:"lastWeighIn"`
	Sets        []sessionSetDTO `json:"sets"`
}

// weighInDTO pairs a bodyweight with the day it was recorded, so a client
// showing "carried from <date>" cannot pair the number with the wrong day.
type weighInDTO struct {
	WeightLb    float64 `json:"weightLb"`
	PerformedOn string  `json:"performedOn"`
}

type sessionExerciseWeightDTO struct {
	ExerciseName string  `json:"exerciseName"`
	Sets         int64   `json:"sets"`
	Reps         int32   `json:"reps"`
	WeightLb     float64 `json:"weightLb"`
}

type sessionSummaryDTO struct {
	ID                int32  `json:"id"`
	ProgramID         int32  `json:"programId"`
	ProgramName       string `json:"programName"`
	ProgramDayID      int32  `json:"programDayId"`
	ProgramDayName    string `json:"programDayName"`
	PerformedOn       string `json:"performedOn"`
	SetCount          int64  `json:"setCount"`
	CompletedSetCount int64  `json:"completedSetCount"`
	// VolumeLb is the weight actually moved this session — actualReps × weightLb
	// summed over the logged sets. A set logged short of its target still counts
	// what it lifted, so this deliberately does not track CompletedSetCount.
	VolumeLb  float64                    `json:"volumeLb"`
	IsOver    bool                       `json:"isOver"`
	Exercises []sessionExerciseWeightDTO `json:"exercises"`
}

type sessionListDTO struct {
	Items []sessionSummaryDTO `json:"items"`
	Total int64               `json:"total"`
	// TotalVolumeLb spans every session matching the filter, not just the ones on
	// this page, so it reads as a lifetime total and holds still while the caller
	// pages through their history.
	TotalVolumeLb float64 `json:"totalVolumeLb"`
	Limit         int32   `json:"limit"`
	Offset        int32   `json:"offset"`
}

// ---- Racked ----
//
// The recap's wire types. They mirror racked.Report one for one: the statistics
// are computed once, server-side, so that this endpoint and the monthly recap
// email render the same numbers. Nothing here is re-aggregated by the client.

type rackedReportDTO struct {
	Period     rackedPeriodDTO     `json:"period"`
	Totals     rackedTotalsDTO     `json:"totals"`
	Change     *rackedChangeDTO    `json:"change"`
	Comparison rackedComparisonDTO `json:"comparison"`
	Split      rackedSplitDTO      `json:"split"`
	// Muscles carries every group in the taxonomy, trained or not — a group with
	// nothing against it is the row worth reading. Always present, and empty
	// only for a period that logged no work at all.
	Muscles []rackedMuscleSliceDTO `json:"muscles"`
	Lifts   []rackedLiftSliceDTO   `json:"lifts"`
	Series  []rackedSeriesDTO      `json:"series"`
	// MostImproved is nil until some lift has been performed twice in the period.
	MostImproved *rackedImprovementDTO `json:"mostImproved"`
	// Bodyweight is nil when the period holds no weigh-in, which is the common
	// case — recording one is optional on every session.
	Bodyweight *rackedBodyweightDTO `json:"bodyweight"`
	Days       []rackedDayVolumeDTO `json:"days"`
	// Weekdays is indexed 0 = Sunday, matching programDay.weekday.
	Weekdays    []float64 `json:"weekdays"`
	BestWeekday int       `json:"bestWeekday"`
	Hours       []int     `json:"hours"`
	// PeakHour is the hour hourLabel describes, so a surface accents the same
	// bar the label names instead of picking its own out of a tie.
	PeakHour  int    `json:"peakHour"`
	HourLabel string `json:"hourLabel"`

	Streak     rackedStreakDTO      `json:"streak"`
	Attendance rackedAttendanceDTO  `json:"attendance"`
	PRs        []rackedPRDTO        `json:"prs"`
	Milestones []rackedMilestoneDTO `json:"milestones"`

	HeaviestSet *rackedSetHighlightDTO `json:"heaviestSet"`
	// FastestSession is nil when nothing in the period was finished by hand — an
	// unfinished session has no duration worth reporting.
	FastestSession *rackedSessionHighlightDTO `json:"fastestSession"`
	Deloads        []rackedDeloadDTO          `json:"deloads"`
	Archetype      rackedArchetypeDTO         `json:"archetype"`
}

type rackedPeriodDTO struct {
	Kind  string `json:"kind"`
	Start string `json:"start"`
	End   string `json:"end"`
	Label string `json:"label"`
	// InProgress is true while the period is still running. Every rate in the
	// report is then measured over the days so far, and the comparison against
	// the preceding period covers the same stretch of it.
	InProgress bool `json:"inProgress"`
}

type rackedTotalsDTO struct {
	VolumeLb float64 `json:"volumeLb"`
	Sessions int     `json:"sessions"`
	Sets     int     `json:"sets"`
	Reps     int     `json:"reps"`
}

// rackedChangeDTO carries percentages as fractions (0.12 is +12%). A nil
// percentage means the prior figure was zero, where a ratio has no meaning.
type rackedChangeDTO struct {
	VolumeLb    float64  `json:"volumeLb"`
	VolumePct   *float64 `json:"volumePct"`
	Sessions    int      `json:"sessions"`
	SessionsPct *float64 `json:"sessionsPct"`
}

type rackedComparisonDTO struct {
	Count  int     `json:"count"`
	Label  string  `json:"label"`
	UnitLb float64 `json:"unitLb"`
}

// rackedSplitDTO divides volumeLb rather than subtracting from it: main plus
// assistance is the headline total, and the two shares sum to 1.
type rackedSplitDTO struct {
	Main       rackedWorkDTO `json:"main"`
	Assistance rackedWorkDTO `json:"assistance"`
}

type rackedWorkDTO struct {
	VolumeLb float64 `json:"volumeLb"`
	Sets     int     `json:"sets"`
	Reps     int     `json:"reps"`
	Lifts    int     `json:"lifts"`
	// Share is of the period's whole volume, not of this class.
	Share float64 `json:"share"`
}

// rackedBodyweightDTO carries the period's weigh-ins. changeLb and changePct are
// nil with a single reading, following rackedChangeDTO: one weigh-in is a fact,
// not a trend.
type rackedBodyweightDTO struct {
	Points    []rackedWeighInDTO `json:"points"`
	StartLb   float64            `json:"startLb"`
	EndLb     float64            `json:"endLb"`
	LowLb     float64            `json:"lowLb"`
	HighLb    float64            `json:"highLb"`
	ChangeLb  *float64           `json:"changeLb"`
	ChangePct *float64           `json:"changePct"`
}

type rackedWeighInDTO struct {
	PerformedOn string  `json:"performedOn"`
	WeightLb    float64 `json:"weightLb"`
}

// rackedMuscleSliceDTO is one muscle group's share of the period. The counters
// match rackedLiftSliceDTO's, and summed over the list they are the headline —
// a lift belongs to exactly one group, so nothing is double-counted.
type rackedMuscleSliceDTO struct {
	// Group is a taxonomy key ("chest", "back", …), not a label: the page and
	// the email capitalise it themselves rather than being handed prose.
	Group    string  `json:"group"`
	VolumeLb float64 `json:"volumeLb"`
	Sets     int     `json:"sets"`
	Reps     int     `json:"reps"`
	Lifts    int     `json:"lifts"`
	Share    float64 `json:"share"`
	Trained  bool    `json:"trained"`
}

type rackedLiftSliceDTO struct {
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
	VolumeLb     float64 `json:"volumeLb"`
	Sets         int     `json:"sets"`
	Reps         int     `json:"reps"`
	Share        float64 `json:"share"`
	// IsAssistance is true only when every set of the lift in the period was
	// assistance. Read split for figures that have to add up.
	IsAssistance bool `json:"isAssistance"`
}

type rackedSeriesPointDTO struct {
	PerformedOn string  `json:"performedOn"`
	TopWeightLb float64 `json:"topWeightLb"`
	E1RMLb      float64 `json:"e1rmLb"`
}

type rackedSeriesDTO struct {
	ExerciseID   int32                  `json:"exerciseId"`
	ExerciseName string                 `json:"exerciseName"`
	IsAssistance bool                   `json:"isAssistance"`
	Points       []rackedSeriesPointDTO `json:"points"`
}

type rackedImprovementDTO struct {
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
	FromLb       float64 `json:"fromLb"`
	ToLb         float64 `json:"toLb"`
	GainLb       float64 `json:"gainLb"`
	GainPct      float64 `json:"gainPct"`
}

type rackedDayVolumeDTO struct {
	Date     string  `json:"date"`
	VolumeLb float64 `json:"volumeLb"`
	Sessions int     `json:"sessions"`
}

type rackedStreakDTO struct {
	LongestWeeks int `json:"longestWeeks"`
	CurrentWeeks int `json:"currentWeeks"`
}

// rackedAttendanceDTO reports what it was measured against alongside the
// number, because the honest denominator depends on whether the lifter ever
// scheduled their program days — most have not.
type rackedAttendanceDTO struct {
	Basis    string  `json:"basis"`
	Expected int     `json:"expected"`
	Actual   int     `json:"actual"`
	Rate     float64 `json:"rate"`
	// SessionsPerWeek is the figure to show when Basis is "none", which is most
	// of the time: it is a measurement rather than a score against a target
	// nobody entered.
	SessionsPerWeek float64 `json:"sessionsPerWeek"`
}

type rackedPRDTO struct {
	Kind         string  `json:"kind"`
	PerformedOn  string  `json:"performedOn"`
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
	WeightLb     float64 `json:"weightLb"`
	Reps         int     `json:"reps"`
	ValueLb      float64 `json:"valueLb"`
	PreviousLb   float64 `json:"previousLb"`
}

type rackedMilestoneDTO struct {
	Kind         string  `json:"kind"`
	PerformedOn  string  `json:"performedOn"`
	Label        string  `json:"label"`
	ValueLb      float64 `json:"valueLb"`
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
}

type rackedSetHighlightDTO struct {
	PerformedOn  string  `json:"performedOn"`
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
	WeightLb     float64 `json:"weightLb"`
	Reps         int     `json:"reps"`
}

type rackedSessionHighlightDTO struct {
	SessionID       int32   `json:"sessionId"`
	PerformedOn     string  `json:"performedOn"`
	ProgramDayName  string  `json:"programDayName"`
	DurationSeconds int     `json:"durationSeconds"`
	VolumeLb        float64 `json:"volumeLb"`
	Sets            int     `json:"sets"`
}

type rackedDeloadDTO struct {
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
	PerformedOn  string  `json:"performedOn"`
	FromLb       float64 `json:"fromLb"`
	ToLb         float64 `json:"toLb"`
	Recovered    bool    `json:"recovered"`
	RecoveredOn  *string `json:"recoveredOn"`
}

type rackedArchetypeDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
