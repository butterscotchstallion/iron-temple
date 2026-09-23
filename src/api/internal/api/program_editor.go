package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Building, renaming and retiring a program of your own.
//
// Every handler here resolves the program through ownedProgram, which scopes on
// created_by_user_id = the caller. That predicate does two jobs at once, as
// DeleteExercise's does: it keeps one lifter out of another's program, and it
// makes the SEEDED catalogue untouchable by everybody — those rows have NULL
// there, and NULL = anything is never true in SQL. So "the install's programs
// are nobody's to edit" needs no check of its own and cannot be forgotten.
//
// Somebody else's program 404s rather than 403s, the rule the rest of this
// package follows: learning that an id is valid is already a leak.

const (
	// Matching maxExerciseNameLen, because both are the same kind of thing — a
	// label a lifter types into a field and then reads back on a card.
	maxProgramNameLen = 80
	// Long enough for a paragraph describing what the program is for, short
	// enough that the picker card stays a card.
	maxProgramDescriptionLen = 500
)

type createProgramRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsShared    bool   `json:"isShared"`
	// CloneFromProgramID starts this program as a copy of another rather than
	// empty. Optional: absent is the blank-page case.
	CloneFromProgramID *int32 `json:"cloneFromProgramId"`
}

func (s *Server) createProgram(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := userFrom(ctx).ID

	var req createProgramRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > maxProgramNameLen {
		badRequest(w, "name must be between 1 and 80 characters")
		return
	}
	description := strings.TrimSpace(req.Description)
	if len(description) > maxProgramDescriptionLen {
		badRequest(w, "description must be 500 characters or fewer")
		return
	}

	// The source, resolved through the VISIBILITY-SCOPED read, so cloning is not
	// a way to see inside somebody else's private program: an id they were never
	// shown comes back as no rows and 404s exactly as fetching it would.
	var source *store.GetProgramRow
	if req.CloneFromProgramID != nil {
		found, err := s.q.GetProgram(ctx, store.GetProgramParams{
			ID: *req.CloneFromProgramID, UserID: userID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			notFound(w, "program not found")
			return
		}
		if err != nil {
			internalError(w)
			return
		}
		// Where the linear-only decision is enforced, and the only place it can
		// be. Madcow's prescription is a set of percentages of a top set, held in
		// program_day_exercise_sets, and the editor has no way to express or edit
		// one — so a clone of it would be a program whose ramps a lifter could
		// see the effects of and never change. Worse, deleting the wrong day
		// would leave a lift with no 100% set and therefore no reference day at
		// all, and the engine would silently fall back to reading every day's
		// history as one series.
		if found.ProgressionKind != progressionKindLinear {
			conflict(w, "unsupported_progression",
				"only linear programs can be copied — this one prescribes per-set ramps")
			return
		}
		source = &found
	}

	if !s.programNameIsFree(w, ctx, name, userID, 0) {
		return
	}

	// One transaction for the whole program. A clone that got its days and lost
	// its prescriptions would be a program that looks right on the picker and
	// prescribes nothing, which is worse than no program at all.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	programID, err := qtx.CreateProgram(ctx, store.CreateProgramParams{
		Name: name, Description: description, UserID: userID, IsShared: req.IsShared,
	})
	if isUniqueViolation(err) {
		// Lost the race past the pre-check above. The index is the guarantee;
		// losing to it is a 409 like any other rather than a 500.
		conflict(w, "duplicate_name", "you already have a program with that name")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	if source != nil {
		if err := qtx.CloneProgramDays(ctx, store.CloneProgramDaysParams{
			TargetProgramID: programID, SourceProgramID: source.ID,
		}); err != nil {
			internalError(w)
			return
		}
		if err := qtx.ClonePrescriptions(ctx, store.ClonePrescriptionsParams{
			TargetProgramID: programID, SourceProgramID: source.ID,
		}); err != nil {
			internalError(w)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	// The whole program rather than an id, so the client can go straight into
	// the editor — which is where every caller of this is heading — without a
	// second round trip to read back what it just asked for.
	s.writeProgram(w, r, programID, http.StatusCreated)
}

type updateProgramRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsShared    *bool   `json:"isShared"`
}

func (s *Server) updateProgram(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	program, ok := s.ownedProgram(w, r)
	if !ok {
		return
	}
	userID := userFrom(ctx).ID

	var req updateProgramRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > maxProgramNameLen {
			badRequest(w, "name must be between 1 and 80 characters")
			return
		}
		// Excluding this program, so re-saving a form without touching the name
		// is not a conflict with itself.
		if !s.programNameIsFree(w, ctx, name, userID, program.ID) {
			return
		}
		req.Name = &name
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		if len(description) > maxProgramDescriptionLen {
			badRequest(w, "description must be 500 characters or fewer")
			return
		}
		req.Description = &description
	}

	n, err := s.q.UpdateProgram(ctx, store.UpdateProgramParams{
		ID: program.ID, UserID: userID,
		Name: req.Name, Description: req.Description, IsShared: req.IsShared,
	})
	if isUniqueViolation(err) {
		conflict(w, "duplicate_name", "you already have a program with that name")
		return
	}
	if err != nil {
		internalError(w)
		return
	}
	if n == 0 {
		notFound(w, "program not found")
		return
	}

	s.writeProgram(w, r, program.ID, http.StatusOK)
}

// archiveProgram retires a program, and unarchiveProgram brings it back.
//
// A sub-resource rather than DELETE /programs/{id}, because this genuinely
// cannot delete: sessions.program_day_id RESTRICTs, so a program somebody has
// trained is not removable, and cascading through to logged sets would trade a
// tidy catalogue for destroyed history. A DELETE that does not delete is a lie
// about what happened, and the lifter would find the program still in their
// history wondering which of the two screens was wrong.
//
// It is also not a field on the metadata PATCH. That would let a client set an
// arbitrary timestamp, and would fold "your rename collided" and "your archive
// failed" into one response the UI has to disentangle.
func (s *Server) archiveProgram(w http.ResponseWriter, r *http.Request) {
	s.setProgramArchived(w, r, true)
}

func (s *Server) unarchiveProgram(w http.ResponseWriter, r *http.Request) {
	s.setProgramArchived(w, r, false)
}

func (s *Server) setProgramArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	ctx := r.Context()
	program, ok := s.ownedProgram(w, r)
	if !ok {
		return
	}

	n, err := s.q.SetProgramArchived(ctx, store.SetProgramArchivedParams{
		ID: program.ID, UserID: userFrom(ctx).ID, Archived: archived,
	})
	if err != nil {
		internalError(w)
		return
	}
	if n == 0 {
		notFound(w, "program not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ownedProgram resolves {programId} and confirms the caller owns it, writing the
// 404 itself so callers can `if !ok { return }`.
//
// Reads through the visibility-scoped GetProgram first and then checks the
// owner, rather than one query that does both, so that "does not exist",
// "cannot see it" and "can see it but it is not yours" all end at the same
// answer without three branches saying so.
func (s *Server) ownedProgram(w http.ResponseWriter, r *http.Request) (store.GetProgramRow, bool) {
	id, ok := idParam(r, "programId")
	if !ok {
		notFound(w, "program not found")
		return store.GetProgramRow{}, false
	}

	ctx := r.Context()
	userID := userFrom(ctx).ID
	program, err := s.q.GetProgram(ctx, store.GetProgramParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "program not found")
		return store.GetProgramRow{}, false
	}
	if err != nil {
		internalError(w)
		return store.GetProgramRow{}, false
	}

	// A seeded program has NULL here, so this is false for every caller — which
	// is the whole of "the install's catalogue is nobody's to edit", expressed
	// once rather than as a rule each handler remembers.
	if program.CreatedByUserID == nil || *program.CreatedByUserID != userID {
		notFound(w, "program not found")
		return store.GetProgramRow{}, false
	}
	return program, true
}

// programNameIsFree reports whether a name is available to this owner, writing
// the 409 itself when it is not. excludingID skips one program, so a rename that
// leaves the name alone does not collide with itself; pass 0 when creating.
//
// Checked here as well as enforced by 0029's partial indexes, because the two
// answer different questions: the index stops the write, and this says which
// write it stopped so the UI can put the message beside the field. The index
// stays the guarantee — a race past this still fails, and fails safely.
func (s *Server) programNameIsFree(
	w http.ResponseWriter, ctx context.Context, name string, userID, excludingID int32,
) bool {
	conflicts, err := s.q.CountProgramNameConflicts(ctx, store.CountProgramNameConflictsParams{
		Name: name, UserID: userID, ExcludingID: excludingID,
	})
	if err != nil {
		internalError(w)
		return false
	}
	if conflicts > 0 {
		conflict(w, "duplicate_name", "you already have a program with that name")
		return false
	}
	return true
}
