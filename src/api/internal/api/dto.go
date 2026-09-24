package api

// Response DTOs. These mirror the OpenAPI schemas exactly (camelCase JSON, plain
// scalar types), keeping the generated sqlc row structs — with their snake_case
// tags and pgtype columns — out of the wire contract.

import "time"

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
	// MustChangePassword is true while the account is still carrying the
	// password an admin chose for it. Always present rather than omitempty: the
	// client branches the whole app on it, and "absent" and "false" have to mean
	// the same thing without the client having to decide which.
	MustChangePassword bool `json:"mustChangePassword"`
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
	// BarWeightLb is what this lifter's bar weighs. Every prescribed weight is
	// loaded onto it, so plate maths and the warm-up ramp are both wrong without
	// it — which is exactly what happened while it was a constant in the UI
	// bundle. Always present: GetBarWeight falls back to 45.
	BarWeightLb float64 `json:"barWeightLb"`
	// Plates is what the lifter owns, heaviest first, and it is finite. Never
	// nil — an account with no plates serializes as [], which the loader reads
	// as bar-only rather than as "unset".
	Plates []plateDTO `json:"plates"`
	// DumbbellStepLb is what this lifter's rack steps by, PER BELL. The pair
	// steps twice it, because every weight this app shows is the whole load —
	// so a rack of 5s moves 10 at a time and has nothing in between.
	//
	// Sent for the same reason the bar is: the client promises a lifter a
	// number before they lift ("the weight goes up 5 lb next time", the weight
	// input's arrows), and it can only promise the one they will actually get
	// if it knows the same grid the engine used. Always present: GetGymSteps
	// falls back to 5.
	DumbbellStepLb float64 `json:"dumbbellStepLb"`
	// The three stacks, sent for the same reason the rack is and carrying the
	// whole load rather than half of it: the client has to be able to name the
	// same weights the engine will. Always present; GetGymSteps falls back to 5
	// for each. See 0028 for why a machine does not step by twice the lightest
	// plate the lifter happens to own.
	MachineStepLb float64 `json:"machineStepLb"`
	CableStepLb   float64 `json:"cableStepLb"`
	BandStepLb    float64 `json:"bandStepLb"`
	// EquipmentConfirmedAt is when the lifter last said this gym was right, or
	// nil if these rows are still the app's guess.
	//
	// Nil is not an error and not a reason to prescribe differently — it drives
	// copy and nothing else (0028). It is a pointer rather than a zero time so
	// that "never confirmed" serializes as null instead of as year 1, which the
	// client would have to know to special-case.
	EquipmentConfirmedAt *time.Time `json:"equipmentConfirmedAt"`
}

// plateDTO is a denomination and how many PAIRS of it are owned. Pairs rather
// than a raw count because plates load symmetrically: three 45s are one usable
// pair, and storing it in the unit it is used in keeps the loader from having to
// halve and round.
type plateDTO struct {
	PlateLb float64 `json:"plateLb"`
	Pairs   int32   `json:"pairs"`
}

// liftBaselineDTO is where one lift starts for one lifter. Only lifts with an
// override appear; the rest fall back to the program's seeded starting weight.
type liftBaselineDTO struct {
	ExerciseID int32   `json:"exerciseId"`
	WeightLb   float64 `json:"weightLb"`
}

type registrationStatusDTO struct {
	Open bool `json:"open"`
}

// adminUserDTO is one row of the admin roster.
//
// Deliberately not userDTO. That one carries the lifter's whole gym — bar,
// plates, dumbbell step — and building it costs four extra queries per user
// (see userDTO in me.go), which is a strange price for a table of names. It
// also has no password field for the same reason userDTO has none: the type
// has nowhere to put one.
//
// CreatedAt is the one thing here that is not on userDTO, and it is what makes
// the list readable as a history of the install: who claimed it, and who was
// added afterwards.
type adminUserDTO struct {
	ID          int32  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	AvatarColor string `json:"avatarColor"`
	// IsAdmin is true for exactly one row, enforced by users_single_admin_idx.
	// Sent anyway rather than inferred from position, because the UI marks that
	// row and should not be re-deriving a fact the server already knows.
	IsAdmin bool `json:"isAdmin"`
	// MustChangePassword tells the admin which accounts have not yet been
	// picked up by the person they were made for — the one-time password is
	// still live, and still known to whoever typed it.
	MustChangePassword bool   `json:"mustChangePassword"`
	CreatedAt          string `json:"createdAt"`
}

// lifterDTO is one lifter as another sees them.
//
// The third account-shaped DTO, and the distinctions between the three are the
// point rather than an accident of growth. userDTO is what you may know about
// yourself and carries the whole gym; adminUserDTO is what the owner may know
// about an account and carries the administrative flags; this is what one lifter
// may know about another, and carries neither.
//
// The gym is left out because it is not somebody else's business in the literal
// sense that it decides nothing for them: another lifter's bar and plates set
// the grid *their* weights land on, and a reader's plate maths must never be
// drawn from them. Leaving them off the type is what makes that mistake
// impossible rather than merely unlikely.
//
// IsAdmin and MustChangePassword are absent for the reason given at length on
// ListLifters: the first answers a question this screen never asked, and the
// second describes a live credential.
type lifterDTO struct {
	ID          int32  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	AvatarColor string `json:"avatarColor"`
	// HasAvatar and AvatarEtag serve the same purpose as on userDTO: tell the UI
	// whether to draw an <img> or the initials chip, and bust its cache when the
	// image changes. Both come off the roster's LEFT JOIN rather than a lookup
	// per row — see ListLifters.
	HasAvatar  bool   `json:"hasAvatar"`
	AvatarEtag string `json:"avatarEtag,omitempty"`
	// LastTrainedOn is the most recent day this lifter logged a rep, omitted
	// when they never have. Omitted rather than zeroed: "" and a date are easy
	// for a client to tell apart, where 1970-01-01 is a date that would draw.
	LastTrainedOn string `json:"lastTrainedOn,omitempty"`
	// Following is whether the CALLER follows this lifter — the one field here that
	// is not a fact about the lifter at all, but about the reader's relationship to
	// them. It is what the Follow button draws from.
	//
	// A POINTER, and that is the whole reason this field needs a comment. lifterDTO
	// is embedded in four other places — a feed entry, a leaderboard row, a
	// comment's author, a notification's actor — and none of their queries ask about
	// follows. A plain bool would serialize `false` on every one of them: an
	// affirmative statement that you do not follow somebody you may well follow,
	// which is worse than saying nothing. Nil serializes as absent and means "not
	// asked", which is the same absent-means-unknown convention LastTrainedOn uses
	// above, applied to a bool the only way it can be.
	//
	// Populated by the roster and the profile, and nowhere else.
	Following *bool `json:"following,omitempty"`
}

// lifterProfileDTO is a lifter plus their two lifetime figures.
//
// It stops at two on purpose. Streaks, records, per-lift trends and muscle
// splits are not restated here — they come from the Racked report, which is the
// same function over the same rows that the lifter reads on their own page. A
// profile that computed its own streak would be a second opinion about one
// history, and the first time the two disagreed the app would have no way to say
// which was right.
type lifterProfileDTO struct {
	lifterDTO
	// CurrentProgramID is the program this lifter last opened, omitted until
	// they have opened one — the same convention userDTO uses.
	CurrentProgramID *int32 `json:"currentProgramId,omitempty"`
	// SessionCount and LifetimeVolumeLb come from SessionTotals with no program
	// filter, which is the same query and therefore the same definition of a
	// started session that the history page's footer totals.
	SessionCount     int64   `json:"sessionCount"`
	LifetimeVolumeLb float64 `json:"lifetimeVolumeLb"`
}

// feedEntryDTO is one session in the feed.
//
// Deliberately not sessionSummaryDTO with a lifter bolted on. That type carries
// Exercises, whose rows come from a second query keyed on a page of session ids
// (see listSessions) — a feed card draws none of them, and the cheapest way to
// keep it from costing a query per entry is for the type to have nowhere to put
// them. Anything wanting the lifts follows the recap link.
//
// The lifter is nested rather than flattened so a client can pass it straight to
// whatever draws an avatar, which is exactly what the roster gives it.
type feedEntryDTO struct {
	ID     int32     `json:"id"`
	Lifter lifterDTO `json:"lifter"`

	ProgramID      int32  `json:"programId"`
	ProgramName    string `json:"programName"`
	ProgramDayID   int32  `json:"programDayId"`
	ProgramDayName string `json:"programDayName"`
	PerformedOn    string `json:"performedOn"`

	SetCount          int64 `json:"setCount"`
	CompletedSetCount int64 `json:"completedSetCount"`
	// VolumeLb is the weight actually moved, on sessionSummaryDTO's definition
	// and from the same SQL expression: actual_reps rather than the target, over
	// every logged set rather than only the completed ones.
	VolumeLb float64 `json:"volumeLb"`
	IsOver   bool    `json:"isOver"`

	// ReactionCount and CommentCount let a row show that a session landed well.
	// Counts only: the feed is not interactive, because giving a reaction needs
	// the session in front of you and that is the recap the row links to.
	ReactionCount int64 `json:"reactionCount"`
	CommentCount  int64 `json:"commentCount"`
}

// activityStatusDTO is what the generated-activity runner is doing.
//
// MaxLifters and MaxWeeks are sent rather than assumed by the client, so the admin
// screen's inputs cannot offer a number the endpoint will refuse.
type activityStatusDTO struct {
	Running bool `json:"running"`
	// Lifters and TickSeconds describe the RUNNING loop, so both are zero when
	// none is.
	Lifters     int `json:"lifters"`
	TickSeconds int `json:"tickSeconds"`
	// Actions and LastAction are how the screen shows the loop is alive rather
	// than merely flagged as running.
	Actions    int    `json:"actions"`
	LastAction string `json:"lastAction,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	MaxLifters int    `json:"maxLifters"`
	MaxWeeks   int    `json:"maxWeeks"`
	// Roster is every username a backfill could create and a teardown would
	// delete, whether or not any of them exists yet.
	//
	// Published so the confirmation on the admin screen can list them. Teardown
	// matches on name rather than on origin — the accounts carry no marker, so it
	// cannot do otherwise — which means an account the owner made by hand under one
	// of these names would be deleted with all its history. Showing the names is
	// the only safeguard against that, so this is load-bearing rather than
	// informational.
	Roster []string `json:"roster"`
}

// activityScheduleDTO is the unattended daily run's settings and its last result.
//
// The last run's figures ride along rather than needing a second endpoint: the only
// screen that reads the schedule is the one that wants to know whether it is
// actually doing anything, and "enabled" alone does not answer that.
type activityScheduleDTO struct {
	Enabled bool `json:"enabled"`
	Lifters int  `json:"lifters"`
	// LastRunOn and its counts are absent until a day has been generated, which is
	// the ordinary state of a schedule just switched on rather than an error.
	LastRunOn     string `json:"lastRunOn,omitempty"`
	LastSessions  int    `json:"lastSessions,omitempty"`
	LastReactions int    `json:"lastReactions,omitempty"`
	LastComments  int    `json:"lastComments,omitempty"`
}

// activitySummaryDTO is what one backfill did.
//
// Accounts counts the ones it CREATED, not the ones it used: a second backfill
// adds history to the lifters the first one made, and reporting four again would
// read as four more people.
type activitySummaryDTO struct {
	Accounts  int `json:"accounts"`
	Sessions  int `json:"sessions"`
	Reactions int `json:"reactions"`
	Comments  int `json:"comments"`
}

// activityTeardownDTO is what a teardown removed — rows actually deleted rather
// than names attempted, which differ whenever an account was never created.
type activityTeardownDTO struct {
	Removed int64 `json:"removed"`
}

// leaderboardDTO is every board for one period.
//
// All of them in one response rather than one per request: a single pass of
// buildRacked per lifter computes every figure, so a per-metric endpoint would
// re-run the heaviest query in the app to read a different field off the same
// result. See leaderboard.go.
type leaderboardDTO struct {
	Period rackedPeriodDTO       `json:"period"`
	Boards []leaderboardBoardDTO `json:"boards"`
}

// leaderboardBoardDTO ranks the lifters by one metric.
//
// Generic rather than a named field per metric, so a surface renders one component
// with a switcher instead of five. Unit is what lets it format a value without
// parsing Metric, and Note is the board's own explanation of what it measures —
// carried on the wire because an attendance board that silently omits the lifters
// with no schedule needs to say so where it is drawn.
type leaderboardBoardDTO struct {
	Metric string `json:"metric"`
	Label  string `json:"label"`
	Unit   string `json:"unit"`
	Note   string `json:"note"`
	// Never nil, so a client branches on length rather than on null.
	Entries []leaderboardEntryDTO `json:"entries"`
}

// leaderboardEntryDTO is one lifter's place on one board.
type leaderboardEntryDTO struct {
	// Rank shares a place between equal figures and skips the ones they used up
	// (1, 2, 2, 4). Computed server-side so every surface agrees, and so a board
	// never invents a difference between two identical numbers.
	Rank   int32     `json:"rank"`
	Lifter lifterDTO `json:"lifter"`
	Value  float64   `json:"value"`
	// Detail names the thing behind the figure where there is one — the lift on
	// the most-improved board. Absent elsewhere.
	Detail string `json:"detail,omitempty"`
}

// achievementDTO is one thing that can be earned.
//
// The catalogue half of the feature: what it is called and what it means, held
// server-side so the five boards and the five crowns derived from them cannot
// drift into describing themselves differently.
type achievementDTO struct {
	Slug string `json:"slug"`
	// Kind is what a client switches on to decide how to draw this. 'crown'
	// today; the column exists because the catalogue is meant to hold more.
	Kind string `json:"kind"`
	// Metric ties a crown back to the leaderboard board it comes from, matching
	// the metric strings on leaderboardBoardDTO. Absent for any kind that is not
	// a board's.
	Metric      string `json:"metric,omitempty"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// achievementHoldersDTO is one achievement and everybody currently wearing it.
//
// Holders is a LIST because the leaderboard gives tied figures the same rank, so
// two lifters genuinely level on a board are both first. Never nil, so a client
// branches on length — an achievement nobody currently holds is an ordinary
// state, not a missing row.
type achievementHoldersDTO struct {
	Achievement achievementDTO `json:"achievement"`
	Holders     []lifterDTO    `json:"holders"`
}

// achievementListDTO is the whole catalogue with its current holders.
//
// THIS IS THE SITE-WIDE READ, and its shape follows from that. Every signed-in
// client asks for it once and then draws from the answer beside every name it
// renders, so it returns everything rather than answering "does this lifter hold
// anything" per lifter — which would be a request per row of the feed.
//
// The catalogue rides along with the holders rather than being a second endpoint
// because a client needs both to draw one crown, and an achievement nobody holds
// still has to be listable on a profile that has never earned it.
type achievementListDTO struct {
	Items []achievementHoldersDTO `json:"items"`
}

// lifterAchievementDTO is one lifter's history with one achievement.
//
// Folded to one entry per achievement rather than one per reign: "held this three
// times" is the fact a profile wants, and a lifter who has traded a crown back and
// forth for a year would otherwise be a list nobody reads to the end of.
type lifterAchievementDTO struct {
	Achievement achievementDTO `json:"achievement"`
	// HeldNow is whether one of these reigns is still open. It is what decides
	// whether the entry is drawn as a possession or as a memory.
	HeldNow bool `json:"heldNow"`
	// TimesHeld counts reigns, not hours. A continuous stretch is one however
	// long it lasts, which is what makes this a number worth showing.
	TimesHeld int64 `json:"timesHeld"`
	// FirstHeldFrom is when they first took it, ever. LastHeldFrom is when the
	// most recent reign began — the same value on a lifter who has held it once.
	// Both are sent because they answer different questions ("since when" for a
	// current reign, "when last" for a lapsed one) and the client picks by
	// HeldNow rather than asking twice.
	FirstHeldFrom string `json:"firstHeldFrom"`
	LastHeldFrom  string `json:"lastHeldFrom"`
}

// lifterAchievementListDTO is one lifter's profile section.
//
// Holds only what they have actually earned. A catalogue entry they have never
// held is absent rather than present-and-empty: the client already has the full
// catalogue from the endpoint above if it wants to draw the ones still to win,
// and repeating it per lifter would put the whole catalogue on every profile.
type lifterAchievementListDTO struct {
	Items []lifterAchievementDTO `json:"items"`
}

// sessionReactionDTO is one emoji's worth of applause.
type sessionReactionDTO struct {
	Emoji string `json:"emoji"`
	Count int64  `json:"count"`
	// Mine lets a surface draw the caller's own tap as pressed. Computed in the
	// grouping query rather than by a second request whose answer would have to be
	// joined back onto this one.
	Mine bool `json:"mine"`
}

// sessionCommentDTO is one comment with its author.
//
// The author is a lifterDTO, which is the third place that type has earned its
// keep: it carries an avatar and carries nothing administrative, and a comment is
// rendered exactly the way a roster row and a feed row are.
type sessionCommentDTO struct {
	ID        int32     `json:"id"`
	SessionID int32     `json:"sessionId"`
	Author    lifterDTO `json:"author"`
	Body      string    `json:"body"`
	CreatedAt string    `json:"createdAt"`
}

// sessionCommentListDTO is a page of a conversation.
//
// It HAS a total, unlike feedDTO, and the difference is what the surface needs
// to say. A feed pages until a short page stops it and never has to announce
// how much is left; a conversation showing its last twenty lines wants to offer
// "17 earlier comments", and a short page cannot count the ones above it.
//
// Items are oldest-first even though Offset counts back from the newest — see
// ListSessionComments for why the two ends differ.
type sessionCommentListDTO struct {
	Items  []sessionCommentDTO `json:"items"`
	Total  int64               `json:"total"`
	Limit  int32               `json:"limit"`
	Offset int32               `json:"offset"`
}

// feedDTO is a page of the feed.
//
// No total, unlike sessionListDTO. "How many sessions have the others ever
// logged" is not a question this surface asks, and answering it would cost a
// second aggregate on every page; a caller pages until a short page stops them.
type feedDTO struct {
	Items  []feedEntryDTO `json:"items"`
	Limit  int32          `json:"limit"`
	Offset int32          `json:"offset"`
}

// notificationDTO is one thing that happened, addressed to the caller.
//
// ONE OF THESE IS A GROUP. ListNotificationGroups folds every notification about
// the same subject into a single row, so this carries the newest member of that
// group plus the two fields that say what else it folded. ID is that member's
// id, and marking it read marks the whole group — see markNotificationRead.
//
// Almost everything below the actor is a pointer, and that is the kind system
// carrying a shape the schema cannot: a 'joined' has no session, only a
// 'reaction' has an emoji, and only a 'comment' or 'reply' has a body. They are
// omitempty so a row on the wire is the fields that mean something for its kind
// and nothing else — a client switches on Kind rather than probing for nulls.
//
// The actor is a lifterDTO for the reason sessionCommentDTO's author is: every
// one of these is drawn as somebody's avatar next to somebody's name, which is
// what that type is for. On a group it is the most recent of them, which is the
// one whose avatar the row draws.
type notificationDTO struct {
	ID    int32     `json:"id"`
	Kind  string    `json:"kind"`
	Actor lifterDTO `json:"actor"`

	// ActorCount is how many DISTINCT people this row folds, Actor included, so
	// it is 1 on a row that folds nothing. Sent unconditionally rather than
	// omitempty: a client building a sentence needs to know it is 1 rather than
	// having to treat absent as 1.
	ActorCount int32 `json:"actorCount"`
	// OtherActorNames names some of the rest, most recent first, for a row that
	// wants to say "Bob, Cara and 4 others". Never includes Actor's own name.
	//
	// Capped at two, and the cap is a WIRE BUDGET rather than a copy decision —
	// an install with forty lifters must not put forty names on every row of
	// every poll. How many to actually say is the client's call, and it can
	// change its mind without changing this contract. Absent when the row folds
	// nobody.
	OtherActorNames []string `json:"otherActorNames,omitempty"`

	SessionID *int32 `json:"sessionId,omitempty"`
	// SessionOwnerID is usually the caller and deliberately not assumed to be:
	// a 'reply' reaches somebody who joined a conversation on a session that was
	// never theirs. The client compares it against its own id to decide which of
	// the two recap routes a row points at — see the note in openapi.yaml about
	// why that is a routing decision and not the API's to make.
	SessionOwnerID *int32  `json:"sessionOwnerId,omitempty"`
	ProgramDayName *string `json:"programDayName,omitempty"`
	Emoji          *string `json:"emoji,omitempty"`
	// CommentID is which comment, so a client can take the lifter to the
	// sentence rather than to the page holding it. The recap a notification
	// points at is long and the conversation is the last card on it.
	CommentID   *int32  `json:"commentId,omitempty"`
	CommentBody *string `json:"commentBody,omitempty"`

	// AchievementSlug is which achievement, for 'crown'. It names the board that
	// was won, so the row can read "took the crown on Week streak" rather than
	// the vaguer thing.
	//
	// Absent when the row folds crowns from MORE THAN ONE board, which is a state
	// only this kind can reach: 'crown' has no session, so every crown on the
	// install folds into one group. Naming the representative's board there would
	// be a claim about the group that the group does not support, so the query
	// withholds it and the client says "took crowns" instead. Absent on every
	// other kind, which has no achievement at all.
	AchievementSlug *string `json:"achievementSlug,omitempty"`

	CreatedAt string `json:"createdAt"`
	// ReadAt is absent while unread, which is the state the badge counts.
	ReadAt string `json:"readAt,omitempty"`
}

// notificationListDTO is a page of the panel.
//
// No total, following feedDTO — but UnreadCount is not that total in disguise.
// It counts the caller's unread notifications across all of them rather than
// the ones on this page, because it is what the header badge draws and the
// header has not asked for a page in any meaningful sense: it asks for this
// endpoint once a minute and mostly gets a 304.
type notificationListDTO struct {
	Items       []notificationDTO `json:"items"`
	Limit       int32             `json:"limit"`
	Offset      int32             `json:"offset"`
	UnreadCount int64             `json:"unreadCount"`
}

// notificationMemberListDTO is one row of the panel, unfolded.
//
// No limit, offset or total, unlike notificationListDTO. This is not a page of
// anything — it is the whole of one group, which the caller has already been told
// the size of by the row they opened. Handing back a page of a row's own contents
// would be a list the client could not reconcile with the sentence above it.
type notificationMemberListDTO struct {
	Items []notificationDTO `json:"items"`
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

// How a program decides next session's weights, mirroring programs
// .progression_kind and the ProgressionKind enum in the spec.
//
// Only linear is named, because it is the only one anything BRANCHES on: a
// program a lifter builds is always linear, and the check that enforces it needs
// a constant to compare against. Madcow is read from the database and passed
// straight to the wire, so naming it here would add a constant nothing reads.
const progressionKindLinear = "linear"

type exerciseDTO struct {
	ID          int32  `json:"id"`
	Name        string `json:"name"`
	MuscleGroup string `json:"muscleGroup"`
	Equipment   string `json:"equipment"`
	IsAccessory bool   `json:"isAccessory"`
	// IsCustom marks a movement this lifter created, which is the only kind
	// anyone may delete.
	IsCustom bool `json:"isCustom"`
	// RestSeconds is how long this movement rests between sets. On the list
	// because a client adding it to a workout has to know the rest before the
	// server has answered — assistance added at the rack without a signal shows
	// its sets straight away, and a default would start three minutes on curls.
	RestSeconds int32 `json:"restSeconds"`
	// TopSet is this lifter's heaviest working set on the movement, or nil if
	// they have never performed it. A pointer rather than a zero value because
	// the field is `required` and nullable in the spec: the key is always
	// present, and null is meaningfully different from a set at zero pounds.
	TopSet *exerciseTopSetDTO `json:"topSet"`
	// LastPerformedOn and PerformedSessions rank the assistance picker: the
	// movements a lifter actually trains, most recent first, so the six they
	// always add are the first thing in the box rather than something to search
	// the catalogue for. A pointer for the date because "never performed" is
	// null and not a zero date; a plain int for the count because zero sessions
	// is a real answer.
	LastPerformedOn   *string `json:"lastPerformedOn"`
	PerformedSessions int32   `json:"performedSessions"`
}

// exerciseTopSetDTO is the heaviest set a lifter has worked on one movement.
// Carried on the list row so the Progress page can render a card per lift
// without fetching each one's whole history to take a maximum from it.
type exerciseTopSetDTO struct {
	WeightLb    float64 `json:"weightLb"`
	PerformedOn string  `json:"performedOn"`
}

// exerciseHistoryDTO carries the lift's identity alongside its history, so a
// caller charting one movement needs no second request to find out what it is
// called. Points is never nil — a lift that exists but has never been performed
// serializes as [], which is different from the 404 an unknown lift now gets.
type exerciseHistoryDTO struct {
	ExerciseID   int32                     `json:"exerciseId"`
	ExerciseName string                    `json:"exerciseName"`
	Points       []exerciseHistoryPointDTO `json:"points"`
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
	// OwnerID is nil for the seeded programs, which belong to the install rather
	// than to anybody — and which is why nobody can edit them: every write scopes
	// on created_by_user_id = caller, and NULL = anything is never true. The same
	// trick deleteExercise uses to make the seeded catalogue undeletable.
	OwnerID *int32 `json:"ownerId"`
	// OwnerName attributes a shared program on the picker without a lookup per
	// card. Empty for the seeded ones, which have nobody to attribute.
	OwnerName string `json:"ownerName,omitempty"`
	// IsMine is editability, and the client should read it as exactly that rather
	// than comparing OwnerID against /me. Derived server-side so there is one
	// definition of ownership and it is the one the write handlers enforce.
	IsMine bool `json:"isMine"`
	// IsShared is whether the rest of the install can FIND this program. It is
	// not whether they can read it: somebody who has trained it keeps access when
	// it is un-shared, because their sessions are bound to its days either way.
	IsShared bool `json:"isShared"`
	// ArchivedAt is nil for a live program. Like IsShared this governs discovery
	// and not access — an archived program is gone from the picker but still
	// resolves, still previews and can still be trained, so a lifter part-way
	// through one is never stranded.
	ArchivedAt *string `json:"archivedAt"`
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
	// Exercises is the program's own prescription — the same for every account
	// training this program, and writable only by whoever owns it: nobody, on the
	// seeded ones. Assistance is the calling lifter's overlay on top of it, on
	// any program.
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
	// RepMin and RepMax turn this lift onto double progression: add reps inside
	// the range week to week, and when every set reaches the top the weight goes
	// up and the reps reset to the bottom. Both absent means the lift carries its
	// weight forward and nothing moves it.
	RepMin *int32 `json:"repMin,omitempty"`
	RepMax *int32 `json:"repMax,omitempty"`
	// Equipment is the movement's, copied from the exercise rather than stored
	// on the overlay. The client needs it for the same reason the engine does:
	// the smallest jump this lift can make is a fact about the bar or the bells
	// it uses, and the program page both offers that jump on a stepper and
	// promises it in words.
	Equipment string `json:"equipment"`
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

// prescribedSessionsDTO is every day of a program prescribed at once.
//
// The layoff sits here rather than on each day because it describes the lifter,
// not the day: the per-day endpoint gives every day an identical copy, and the
// client kept whichever answer landed last. Hoisting it removes the question of
// which one was authoritative.
type prescribedSessionsDTO struct {
	ProgramID int32              `json:"programId"`
	Layoff    *layoffDTO         `json:"layoff"`
	Days      []prescribedDayDTO `json:"days"`
}

// prescribedDayDTO is prescribedSessionDTO without the fields its wrapper
// already carries.
type prescribedDayDTO struct {
	ProgramDayID   int32                   `json:"programDayId"`
	ProgramDayName string                  `json:"programDayName"`
	Exercises      []prescribedExerciseDTO `json:"exercises"`
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
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
	Kind         string  `json:"kind"`
	Sets         int32   `json:"sets"`
	Reps         int32   `json:"reps"`
	WeightLb     float64 `json:"weightLb"`
	RestSeconds  int32   `json:"restSeconds"`
	// RepMin and RepMax bound an assistance lift's rep range, when it has one.
	// Reps above is the bottom of that range — a set is complete at the bottom
	// and the weight moves at the top — so the UI needs both to render "3x8-12"
	// rather than a bare "3x8". Absent on main lifts and on assistance the
	// lifter has not put a range on.
	RepMin *int32 `json:"repMin,omitempty"`
	RepMax *int32 `json:"repMax,omitempty"`
	// SetPlan is every set this lift prescribes today, with its own reps and its
	// own weight. Always populated, for every program: a lift with one weight
	// across its sets emits a flat plan rather than nothing, so a client has one
	// shape to render instead of two.
	//
	// Sets and WeightLb above stay meaningful alongside it — Sets is len(SetPlan)
	// and WeightLb is the top set, which is the number that moves week to week
	// and the one to put beside the lift's name.
	SetPlan     []prescribedSetDTO `json:"setPlan"`
	Progression progressionInfoDTO `json:"progression"`
}

// prescribedSetDTO is one set of a prescription. Ramping programs give each set
// its own weight and reps — Madcow climbs 50/62.5/75/87.5/100% of a top set, and
// its intensity day finishes with a triple above that and a backoff below it.
type prescribedSetDTO struct {
	SetNumber int32   `json:"setNumber"`
	Reps      int32   `json:"reps"`
	WeightLb  float64 `json:"weightLb"`
}

// progressionInfoDTO explains why the engine chose a lift's weight, so the UI
// can surface an impending stall or a deload instead of a bare number.
type progressionInfoDTO struct {
	// Status is one of the progression.Status values
	// (start|advance|hold|deload|layoff), or, for assistance, "fixed" when the
	// weight was carried forward and "progressing" when a rep range advanced it.
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
	// IsBonus marks a set the lifter added after working through everything else
	// in the session — extra work, rather than the workout as it was planned.
	//
	// "Worked through" is not "hit every target": a set logged below its target
	// is one the lifter is done with, and does not hold back the bonus flag. See
	// AppendSessionSet, which reads actual_reps rather than completed for
	// exactly that reason.
	//
	// Recorded when the set is appended and never recomputed; see 0027 for why
	// it cannot be. Always false for sets a session opened with, and for every
	// set logged before the column existed.
	IsBonus     bool  `json:"isBonus"`
	RestSeconds int32 `json:"restSeconds"`
	// Equipment is the movement's, carried on every set because the session
	// screen is the one screen that never loads the exercise library. Two things
	// there are wrong without it: the warm-up ramp opens with two sets of an
	// empty bar and rounds its rungs onto plates, and the weight stepper moves by
	// a bar's 5 lb. On a pair of dumbbells the first describes equipment nobody
	// is holding and the second builds a 35 lb pair out of 5 lb bells.
	Equipment string `json:"equipment"`
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
	LastWeighIn *weighInDTO `json:"lastWeighIn"`
	// PreviousBests is the record to beat for each lift in this session, from
	// every OTHER session. Never nil — a lifter with no history serializes as
	// [], which reads as "nothing to beat" rather than "not told".
	PreviousBests []personalBestDTO `json:"previousBests"`
	Sets          []sessionSetDTO   `json:"sets"`
}

// personalBestDTO is one lift's heaviest working weight before this session.
// Carried on the session so the screen can flag a personal record without
// fetching a full history per exercise.
type personalBestDTO struct {
	ExerciseID int32   `json:"exerciseId"`
	WeightLb   float64 `json:"weightLb"`
	// E1rmLb is the estimated-max mark to beat. The live screen flags only the
	// weight; the recap reports both kinds of record, and reconstructs itself
	// from this response when it cannot reach the server.
	E1rmLb float64 `json:"e1rmLb"`
}

// ---- Session recap ----
// One workout in review. Reuses rackedPRDTO, rackedMilestoneDTO and
// rackedComparisonDTO rather than restating them, which is what keeps a record
// announced at the rack and the same record listed in March's recap one object.

type sessionRecapDTO struct {
	Session sessionRecapHeaderDTO `json:"session"`
	// DurationSeconds is nil when the lifter never tapped Finish, and nil when
	// the session ran past the 12-hour cap — an overnight tab measures itself,
	// not the training.
	DurationSeconds *int                    `json:"durationSeconds"`
	Pace            *sessionRecapPaceDTO    `json:"pace"`
	Volume          sessionRecapVolumeDTO   `json:"volume"`
	Progress        sessionRecapProgressDTO `json:"progress"`
	// Muscles carries only the groups this session trained — unlike the monthly
	// recap, where an untrained group is the finding and gets a row of its own.
	Muscles []rackedMuscleSliceDTO `json:"muscles"`
	Split   rackedSplitDTO         `json:"split"`
	// BodyweightLb is nil when the lifter did not step on a scale, which is a
	// different answer from any number.
	BodyweightLb *float64 `json:"bodyweightLb"`
	// Earned is nil on any but the lifter's most recent session of the day —
	// see recapEarned for why it is withheld rather than qualified.
	Earned *sessionRecapEarnedDTO `json:"earned"`
	// Lifts, PRs and Milestones are never nil: a first workout serializes them
	// as [], which reads as "nothing yet" rather than "not told".
	Lifts      []sessionRecapLiftDTO `json:"lifts"`
	PRs        []rackedPRDTO         `json:"prs"`
	Milestones []rackedMilestoneDTO  `json:"milestones"`
	Streak     sessionRecapStreakDTO `json:"streak"`
}

type sessionRecapHeaderDTO struct {
	SessionID      int32   `json:"sessionId"`
	ProgramID      int32   `json:"programId"`
	ProgramName    string  `json:"programName"`
	ProgramDayID   int32   `json:"programDayId"`
	ProgramDayName string  `json:"programDayName"`
	PerformedOn    string  `json:"performedOn"`
	StartedAt      string  `json:"startedAt"`
	FinishedAt     *string `json:"finishedAt"`
	IsOver         bool    `json:"isOver"`
}

type sessionRecapPaceDTO struct {
	MedianSeconds int `json:"medianSeconds"`
	// DeltaPct is negative when this session was FASTER than usual — the one
	// signed figure in this API where a drop is the good news.
	DeltaPct   float64 `json:"deltaPct"`
	Rank       int     `json:"rank"`
	Of         int     `json:"of"`
	SampleSize int     `json:"sampleSize"`
}

type sessionRecapVolumeDTO struct {
	TotalLb    float64             `json:"totalLb"`
	PreviousLb *float64            `json:"previousLb"`
	DeltaPct   *float64            `json:"deltaPct"`
	Comparison rackedComparisonDTO `json:"comparison"`

	SetsLogged     int `json:"setsLogged"`
	SetsPrescribed int `json:"setsPrescribed"`
	RepsLogged     int `json:"repsLogged"`
	RepsTargeted   int `json:"repsTargeted"`
	SetsBonus      int `json:"setsBonus"`
}

type sessionRecapProgressDTO struct {
	PreviousSessionID   *int32   `json:"previousSessionId"`
	PreviousPerformedOn *string  `json:"previousPerformedOn"`
	WeightDeltaPct      *float64 `json:"weightDeltaPct"`
	LiftsCompared       int      `json:"liftsCompared"`
	LiftsNew            int      `json:"liftsNew"`
}

type sessionRecapLiftDTO struct {
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
	Kind         string  `json:"kind"`
	TopWeightLb  float64 `json:"topWeightLb"`
	TopReps      int     `json:"topReps"`
	TopE1rmLb    float64 `json:"topE1rmLb"`

	SetsLogged     int     `json:"setsLogged"`
	SetsPrescribed int     `json:"setsPrescribed"`
	RepsLogged     int     `json:"repsLogged"`
	RepsTargeted   int     `json:"repsTargeted"`
	SetsBonus      int     `json:"setsBonus"`
	VolumeLb       float64 `json:"volumeLb"`
	HitEveryTarget bool    `json:"hitEveryTarget"`

	Previous       *sessionRecapLiftPreviousDTO `json:"previous"`
	WeightDeltaLb  *float64                     `json:"weightDeltaLb"`
	WeightDeltaPct *float64                     `json:"weightDeltaPct"`
	E1rmDeltaPct   *float64                     `json:"e1rmDeltaPct"`
}

type sessionRecapLiftPreviousDTO struct {
	PerformedOn string  `json:"performedOn"`
	TopWeightLb float64 `json:"topWeightLb"`
	TopReps     int     `json:"topReps"`
	TopE1rmLb   float64 `json:"topE1rmLb"`
}

type sessionRecapStreakDTO struct {
	Sessions int `json:"sessions"`
	Weeks    int `json:"weeks"`
}

// sessionRecapEarnedDTO reuses prescribedExerciseDTO, so what the recap
// promises for next time is the same object the next-session preview returns —
// including its progression block, which is what makes a deload read as a
// decision rather than as an unexplained number a week later.
type sessionRecapEarnedDTO struct {
	ProgramDayID   int32                   `json:"programDayId"`
	ProgramDayName string                  `json:"programDayName"`
	Exercises      []prescribedExerciseDTO `json:"exercises"`
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
	// UpcomingMilestones is the only forward-looking field in this report.
	// Everything else is a reading of what happened; this is what there is to aim
	// at. Never nil, and short or empty rather than padded.
	UpcomingMilestones []rackedUpcomingMilestoneDTO `json:"upcomingMilestones"`

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
	// Weekdays are the days the schedule runs on, for surfaces that draw it
	// rather than grade against it. Empty whenever Basis is "none", so it needs
	// no second check.
	Weekdays []int `json:"weekdays"`
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

// rackedUpcomingMilestoneDTO is a threshold the lifter has not reached yet.
//
// Not a variant of rackedMilestoneDTO, deliberately. That type is shared with the
// session recap, and the two are different facts: a crossed threshold has a date
// and a value, an uncrossed one has a target and a distance. Folding them together
// would put a nullable date on one endpoint and a nullable target on the other,
// and leave every client working out which half it had been handed.
//
// CurrentLb and TargetLb are both sent rather than the remainder, so a surface can
// draw a bar without reconstructing the denominator — and so "20 lb to go" and the
// bar behind it cannot disagree.
type rackedUpcomingMilestoneDTO struct {
	Kind         string  `json:"kind"`
	Label        string  `json:"label"`
	TargetLb     float64 `json:"targetLb"`
	CurrentLb    float64 `json:"currentLb"`
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
