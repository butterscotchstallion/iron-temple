package api

// Response DTOs. These mirror the OpenAPI schemas exactly (camelCase JSON, plain
// scalar types), keeping the generated sqlc row structs — with their snake_case
// tags and pgtype columns — out of the wire contract.

// restSecondsDefault is the prescribed rest between sets. There is no column for
// it in the schema; the spec exposes it with a fixed 3-minute default.
const restSecondsDefault int32 = 180

type errorDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type healthDTO struct {
	Status string `json:"status"`
}

type exerciseDTO struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
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
	ID        int32                   `json:"id"`
	Name      string                  `json:"name"`
	Position  int32                   `json:"position"`
	Exercises []programDayExerciseDTO `json:"exercises"`
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
}

type prescribedExerciseDTO struct {
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
	Sets         int32   `json:"sets"`
	Reps         int32   `json:"reps"`
	WeightLb     float64 `json:"weightLb"`
	RestSeconds  int32   `json:"restSeconds"`
}

type sessionSetDTO struct {
	ID           int32   `json:"id"`
	ExerciseID   int32   `json:"exerciseId"`
	ExerciseName string  `json:"exerciseName"`
	SetNumber    int32   `json:"setNumber"`
	TargetReps   int32   `json:"targetReps"`
	ActualReps   *int32  `json:"actualReps"`
	WeightLb     float64 `json:"weightLb"`
	Completed    bool    `json:"completed"`
	RestSeconds  int32   `json:"restSeconds"`
}

type sessionDTO struct {
	ID             int32           `json:"id"`
	ProgramID      int32           `json:"programId"`
	ProgramName    string          `json:"programName"`
	ProgramDayID   int32           `json:"programDayId"`
	ProgramDayName string          `json:"programDayName"`
	PerformedOn    string          `json:"performedOn"`
	Notes          string          `json:"notes"`
	CreatedAt      string          `json:"createdAt"`
	Sets           []sessionSetDTO `json:"sets"`
}

type sessionExerciseWeightDTO struct {
	ExerciseName string  `json:"exerciseName"`
	Sets         int64   `json:"sets"`
	Reps         int32   `json:"reps"`
	WeightLb     float64 `json:"weightLb"`
}

type sessionSummaryDTO struct {
	ID                int32                      `json:"id"`
	ProgramID         int32                      `json:"programId"`
	ProgramName       string                     `json:"programName"`
	ProgramDayID      int32                      `json:"programDayId"`
	ProgramDayName    string                     `json:"programDayName"`
	PerformedOn       string                     `json:"performedOn"`
	SetCount          int64                      `json:"setCount"`
	CompletedSetCount int64                      `json:"completedSetCount"`
	Exercises         []sessionExerciseWeightDTO `json:"exercises"`
}

type sessionListDTO struct {
	Items  []sessionSummaryDTO `json:"items"`
	Total  int64               `json:"total"`
	Limit  int32               `json:"limit"`
	Offset int32               `json:"offset"`
}
