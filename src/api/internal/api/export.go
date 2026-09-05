package api

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// exportFormatVersion is the shape of the document, not the app's version.
//
// Bumped only when a field is removed or its meaning changes. An importer
// should refuse a number it does not recognise and tolerate fields it does not
// recognise at a version it does — which is what makes adding a field a
// non-event and taking one away a decision.
const exportFormatVersion = 1

// ---- wire shapes ----
//
// Spelled out here rather than reusing the DTOs the rest of the API serves,
// which is a deliberate duplication. Those shapes exist to feed the UI and
// change whenever a screen wants something different; this one is a file that
// lands in somebody's downloads folder and is opened again in three years. They
// have different reasons to change, so they are different types — the day
// sessionDTO grows a field for a new card, this document should not move.

type accountExportDTO struct {
	FormatVersion   int                   `json:"formatVersion"`
	ExportedAt      string                `json:"exportedAt"`
	AppVersion      string                `json:"appVersion"`
	Profile         exportProfileDTO      `json:"profile"`
	Gym             exportGymDTO          `json:"gym"`
	Baselines       []exportBaselineDTO   `json:"baselines"`
	CustomExercises []exportExerciseDTO   `json:"customExercises"`
	Assistance      []exportAssistanceDTO `json:"assistance"`
	Sessions        []exportSessionDTO    `json:"sessions"`
}

type exportProfileDTO struct {
	Username       string  `json:"username"`
	DisplayName    string  `json:"displayName"`
	AvatarColor    *string `json:"avatarColor"`
	CurrentProgram *string `json:"currentProgram"`
	CreatedAt      string  `json:"createdAt"`
}

type exportGymDTO struct {
	BarWeightLb float64    `json:"barWeightLb"`
	Plates      []plateDTO `json:"plates"`
}

type exportBaselineDTO struct {
	ExerciseName string  `json:"exerciseName"`
	WeightLb     float64 `json:"weightLb"`
}

type exportExerciseDTO struct {
	Name        string `json:"name"`
	MuscleGroup string `json:"muscleGroup"`
	Equipment   string `json:"equipment"`
	IsAccessory bool   `json:"isAccessory"`
	RestSeconds int32  `json:"restSeconds"`
	CreatedAt   string `json:"createdAt"`
}

type exportAssistanceDTO struct {
	ProgramName    string  `json:"programName"`
	ProgramDayName string  `json:"programDayName"`
	ExerciseName   string  `json:"exerciseName"`
	Sets           int32   `json:"sets"`
	Reps           int32   `json:"reps"`
	RepMin         *int32  `json:"repMin"`
	RepMax         *int32  `json:"repMax"`
	WeightLb       float64 `json:"weightLb"`
	CreatedAt      string  `json:"createdAt"`
}

type exportSessionDTO struct {
	ID             int32                 `json:"id"`
	ProgramName    string                `json:"programName"`
	ProgramDayName string                `json:"programDayName"`
	PerformedOn    string                `json:"performedOn"`
	Notes          string                `json:"notes"`
	BodyweightLb   *float64              `json:"bodyweightLb"`
	CreatedAt      string                `json:"createdAt"`
	FinishedAt     *string               `json:"finishedAt"`
	Sets           []exportSessionSetDTO `json:"sets"`
}

type exportSessionSetDTO struct {
	ExerciseName string  `json:"exerciseName"`
	Kind         string  `json:"kind"`
	SetNumber    int32   `json:"setNumber"`
	TargetReps   int32   `json:"targetReps"`
	ActualReps   *int32  `json:"actualReps"`
	WeightLb     float64 `json:"weightLb"`
	Completed    bool    `json:"completed"`
}

// exportAccount serves the whole account as one JSON document.
//
// Five queries regardless of how much training is in it. Sets are read for the
// account in a single pass and grouped onto their sessions in memory, because
// the alternative — a query per session — is a few hundred round trips for a
// couple of years of lifting, on the one endpoint where the response is already
// the largest thing the API produces.
//
// Unlike every other read here, a partial answer is not acceptable. getMe will
// happily serve a profile with the gym missing rather than 500, because a
// degraded profile still lets someone train; an export that quietly omits a
// section is a file the lifter keeps believing it is a backup. So every error
// fails the whole request.
func (s *Server) exportAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)

	user, err := s.q.GetUser(ctx, u.ID)
	if err != nil {
		internalError(w)
		return
	}

	doc := accountExportDTO{
		FormatVersion: exportFormatVersion,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		AppVersion:    s.version,
		Profile: exportProfileDTO{
			Username:    user.Username,
			DisplayName: user.DisplayName,
			CreatedAt:   timestamptzToString(user.CreatedAt),
		},
		// Empty slices, not nil. A JSON `null` where an array is declared makes
		// every reader special-case it; `[]` is what "you have none of these"
		// looks like and needs no handling at all.
		Baselines:       []exportBaselineDTO{},
		CustomExercises: []exportExerciseDTO{},
		Assistance:      []exportAssistanceDTO{},
		Sessions:        []exportSessionDTO{},
	}
	if user.AvatarColor != "" {
		colour := user.AvatarColor
		doc.Profile.AvatarColor = &colour
	}
	// The program is named rather than keyed, so it costs a lookup. A program
	// id that no longer resolves is left as null rather than failing the
	// export: it says the lifter has no current program, which is true enough
	// and infinitely better than refusing to hand over the training history.
	if user.CurrentProgramID != nil {
		if program, err := s.q.GetProgram(ctx, *user.CurrentProgramID); err == nil {
			name := program.Name
			doc.Profile.CurrentProgram = &name
		}
	}

	gym, err := s.exportGym(ctx, u.ID)
	if err != nil {
		internalError(w)
		return
	}
	doc.Gym = gym

	baselines, err := s.q.ExportBaselines(ctx, u.ID)
	if err != nil {
		internalError(w)
		return
	}
	for _, b := range baselines {
		doc.Baselines = append(doc.Baselines, exportBaselineDTO{
			ExerciseName: b.ExerciseName,
			WeightLb:     numericToFloat(b.WeightLb),
		})
	}

	exercises, err := s.q.ExportCustomExercises(ctx, u.ID)
	if err != nil {
		internalError(w)
		return
	}
	for _, e := range exercises {
		doc.CustomExercises = append(doc.CustomExercises, exportExerciseDTO{
			Name:        e.Name,
			MuscleGroup: e.MuscleGroup,
			Equipment:   e.Equipment,
			IsAccessory: e.IsAccessory,
			RestSeconds: e.RestSeconds,
			CreatedAt:   timestamptzToString(e.CreatedAt),
		})
	}

	assistance, err := s.q.ExportAssistance(ctx, u.ID)
	if err != nil {
		internalError(w)
		return
	}
	for _, a := range assistance {
		doc.Assistance = append(doc.Assistance, exportAssistanceDTO{
			ProgramName:    a.ProgramName,
			ProgramDayName: a.ProgramDayName,
			ExerciseName:   a.ExerciseName,
			Sets:           a.Sets,
			Reps:           a.Reps,
			RepMin:         a.RepMin,
			RepMax:         a.RepMax,
			WeightLb:       numericToFloat(a.WeightLb),
			CreatedAt:      timestamptzToString(a.CreatedAt),
		})
	}

	sessions, err := s.exportSessions(ctx, u.ID)
	if err != nil {
		internalError(w)
		return
	}
	doc.Sessions = sessions

	// A filename with the date in it, because these accumulate in a downloads
	// folder and "iron-temple-export.json (3)" tells nobody which is current.
	filename := fmt.Sprintf("iron-temple-%s-%s.json",
		user.Username, time.Now().UTC().Format(dateLayout))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	// This is a snapshot of an account, and a copy of it sitting in any cache
	// is a copy nobody asked for. The route is also mounted outside the jsonETag
	// middleware so nothing downstream overrides this — see Router.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, doc)
}

// exportGym reads the bar and the rack. Unlike userDTO's copy of this, a
// failure is returned rather than defaulted: 45 lb is a sane fallback for a
// screen that has to draw something, and a lie in a document claiming to be a
// record of what this lifter owns.
func (s *Server) exportGym(ctx context.Context, userID int32) (exportGymDTO, error) {
	bar, err := s.q.GetBarWeight(ctx, userID)
	if err != nil {
		return exportGymDTO{}, err
	}
	plates, err := s.q.ListPlates(ctx, userID)
	if err != nil {
		return exportGymDTO{}, err
	}

	gym := exportGymDTO{BarWeightLb: numericToFloat(bar), Plates: []plateDTO{}}
	for _, p := range plates {
		gym.Plates = append(gym.Plates, plateDTO{
			PlateLb: numericToFloat(p.PlateLb),
			Pairs:   p.Pairs,
		})
	}
	return gym, nil
}

// exportSessions reads every session and every set in two queries, then nests
// one inside the other.
//
// The grouping is by session id into a map of slices, and the sessions are
// walked in their own query's order afterwards — so the output order is the
// one ExportSessions chose (oldest first), not whatever order the map happens
// to iterate in.
func (s *Server) exportSessions(ctx context.Context, userID int32) ([]exportSessionDTO, error) {
	sessions, err := s.q.ExportSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	sets, err := s.q.ExportSessionSets(ctx, userID)
	if err != nil {
		return nil, err
	}

	bySession := make(map[int32][]exportSessionSetDTO, len(sessions))
	for _, set := range sets {
		bySession[set.SessionID] = append(bySession[set.SessionID], exportSessionSetDTO{
			ExerciseName: set.ExerciseName,
			Kind:         setKind(set.IsAssistance),
			SetNumber:    set.SetNumber,
			TargetReps:   set.TargetReps,
			ActualReps:   set.ActualReps,
			WeightLb:     numericToFloat(set.WeightLb),
			Completed:    set.Completed,
		})
	}

	out := make([]exportSessionDTO, 0, len(sessions))
	for _, row := range sessions {
		out = append(out, exportSessionDTO{
			ID:             row.ID,
			ProgramName:    row.ProgramName,
			ProgramDayName: row.ProgramDayName,
			PerformedOn:    dateToString(row.PerformedOn),
			Notes:          row.Notes,
			BodyweightLb:   optionalNumeric(row.BodyweightLb),
			CreatedAt:      timestamptzToString(row.CreatedAt),
			FinishedAt:     optionalTimestamptz(row.FinishedAt),
			Sets:           setsFor(bySession, row.ID),
		})
	}
	return out, nil
}

// setsFor returns a session's sets, or an empty slice for a session with none —
// a session that was created and never logged against is still part of the
// record, and `null` where an array is declared is a shape readers should not
// have to handle.
func setsFor(bySession map[int32][]exportSessionSetDTO, id int32) []exportSessionSetDTO {
	if sets, ok := bySession[id]; ok {
		return sets
	}
	return []exportSessionSetDTO{}
}
