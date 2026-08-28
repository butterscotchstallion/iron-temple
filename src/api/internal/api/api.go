// Package api implements the Iron Temple HTTP API defined by openapi.yaml.
// Handlers read and write the DTOs in dto.go; persistence goes through the
// sqlc-generated store, and next-session weights come from the progression
// engine.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.homelab/gitadmin/iron-temple/api/internal/auth"
	"gitea.homelab/gitadmin/iron-temple/api/internal/racked"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Server holds the dependencies shared by every handler.
type Server struct {
	pool        *pgxpool.Pool
	q           *store.Queries
	version     string
	environment string
	hasher      auth.PBKDF2Hasher
	// logins brakes password guessing. In-process state, so it is per-replica —
	// see the type's doc for why that is the right trade here.
	logins *auth.RateLimiter
	// reportLoc is the zone the Racked recap reads clock times in. Dates are
	// stored as dates and need no zone; session start times are instants, and
	// "you are an early riser" is a claim about local mornings.
	reportLoc *time.Location
	// mailer delivers the Racked recap. Nil disables the reporter entirely,
	// which is how tests and local development avoid sending real mail — the
	// integration suite drives sendDueReports directly instead.
	mailer *racked.Mailer
}

// NewServer builds a Server over a pgx connection pool. version and environment
// are surfaced by the health endpoint (and the UI header bar); environment also
// decides whether session cookies are marked Secure.
func NewServer(pool *pgxpool.Pool, version, environment string) *Server {
	return &Server{
		pool:        pool,
		q:           store.New(pool),
		version:     version,
		environment: environment,
		logins:      auth.NewRateLimiter(auth.DefaultAttempts, auth.DefaultWindow),
		reportLoc:   time.UTC,
	}
}

// SetReportLocation sets the zone the Racked recap buckets session start times
// in. Defaults to UTC; main.go overrides it from REPORT_TZ. A nil location is
// ignored rather than accepted, so a bad zone name cannot turn every timestamp
// into a panic on the first page load.
func (s *Server) SetReportLocation(loc *time.Location) {
	if loc != nil {
		s.reportLoc = loc
	}
}

// SetMailer gives the server a way to deliver Racked recaps. Until it is
// called, StartRackedReporter does nothing.
func (s *Server) SetMailer(m *racked.Mailer) {
	s.mailer = m
}

func (s *Server) reportLocation() *time.Location {
	if s.reportLoc == nil {
		return time.UTC
	}
	return s.reportLoc
}

// reportToday is the current date in the zone recaps are reported in, as the
// UTC midnight every date in the system is held at.
//
// One reading, used for both the period a recap covers and the point it is
// measured up to. Taking those from different clocks — one UTC, one the report
// zone — lets them land on different calendar days in the hours around a month
// boundary, and a recap whose window sits outside its own period is incoherent
// in every figure derived from it.
func (s *Server) reportToday() time.Time {
	now := time.Now().In(s.reportLocation())
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// Router returns the fully-wired HTTP handler. corsOrigin is a comma-separated
// allowlist of UI origins; empty means allow any origin (dev convenience).
func (s *Server) Router(corsOrigin string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	// AllowCredentials is deliberately absent. The UI is same-origin with the
	// API in both development (the Vite proxy) and production (Traefik path
	// routing), so the session cookie is sent without any CORS involvement.
	// Turning credentials on here — especially alongside the "*" default below
	// — would let any site read authenticated responses.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: corsOrigins(corsOrigin),
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowedHeaders: []string{"Accept", "Content-Type"},
		MaxAge:         300,
	}))
	r.Use(sameOrigin)

	// All paths live under the OpenAPI server base path.
	r.Route("/api/v1", func(r chi.Router) {
		// ---- public ----
		// /health is a Kubernetes probe target and must not need a session.
		r.Get("/health", s.getHealth)
		// Avatars are <img> sources; see getUserAvatar for why they are public.
		r.Get("/users/{userId}/avatar", s.getUserAvatar)

		r.Route("/auth", func(r chi.Router) {
			r.Get("/registration-status", s.getRegistrationStatus)
			r.Post("/register", s.register)
			r.Post("/login", s.login)
			// Logout needs the session it is revoking.
			r.With(s.requireUser).Post("/logout", s.logout)
		})

		// ---- authenticated ----
		// Everything below is per-user or reads per-user history. Mounting it
		// in one Group means a new route is private by default: the mistake to
		// avoid is a handler that quietly sits outside the middleware.
		r.Group(func(r chi.Router) {
			r.Use(s.requireUser)

			r.Route("/me", func(r chi.Router) {
				r.Get("/", s.getMe)
				r.Patch("/", s.updateMe)
				r.Put("/password", s.changePassword)
				r.Post("/avatar", s.uploadAvatar)
				r.Delete("/avatar", s.deleteAvatar)
				r.Get("/baselines", s.listBaselines)
				r.Put("/baselines/{exerciseId}", s.setBaseline)
				r.Delete("/baselines/{exerciseId}", s.clearBaseline)
			})

			r.Route("/exercises", func(r chi.Router) {
				r.Get("/", s.listExercises)
				r.Post("/", s.createExercise)
				r.Delete("/{exerciseId}", s.deleteExercise)
				r.Get("/{exerciseId}/history", s.getExerciseHistory)
			})

			r.Route("/programs", func(r chi.Router) {
				r.Get("/", s.listPrograms)
				r.Get("/{programId}", s.getProgram)
				r.Get("/{programId}/days/{dayId}/next-session", s.previewNextSession)
				r.Patch("/{programId}/days/{dayId}", s.updateProgramDayWeekday)
				// Assistance is per-user state hanging off a shared program day,
				// which is why it is a nested collection rather than a field on
				// the day: it is created and deleted by the caller alone, and
				// the program itself is never written to.
				r.Post("/{programId}/days/{dayId}/assistance", s.addAssistance)
				r.Patch("/{programId}/days/{dayId}/assistance/{assistanceId}", s.updateAssistance)
				r.Delete("/{programId}/days/{dayId}/assistance/{assistanceId}", s.removeAssistance)
			})

			r.Get("/racked", s.getRacked)

			r.Route("/sessions", func(r chi.Router) {
				r.Get("/", s.listSessions)
				r.Post("/", s.createSession)
				r.Get("/{sessionId}", s.getSession)
				r.Patch("/{sessionId}", s.updateSession)
				r.Delete("/{sessionId}", s.deleteSession)
				r.Post("/{sessionId}/finish", s.finishSession)
				r.Patch("/{sessionId}/sets/{setId}", s.updateSessionSet)
			})
		})
	})

	return r
}

func corsOrigins(origin string) []string {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return []string{"*"}
	}
	parts := strings.Split(origin, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// ---- shared response + parsing helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorDTO{Code: code, Message: message})
}

func badRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, "bad_request", message)
}

func notFound(w http.ResponseWriter, message string) {
	writeError(w, http.StatusNotFound, "not_found", message)
}

func unauthorized(w http.ResponseWriter, message string) {
	writeError(w, http.StatusUnauthorized, "unauthenticated", message)
}

func forbidden(w http.ResponseWriter, code, message string) {
	writeError(w, http.StatusForbidden, code, message)
}

// conflict reports a request that is well-formed and permitted but collides with
// state that already exists — a duplicate name, an exercise still in use. It
// takes an explicit code because, unlike the responses above, the caller usually
// wants to tell the cases apart to choose a message.
func conflict(w http.ResponseWriter, code, message string) {
	writeError(w, http.StatusConflict, code, message)
}

// uniqueViolation is Postgres' SQLSTATE for a unique constraint breach. Spelled
// out rather than pulled from jackc/pgerrcode, which is not a dependency here and
// is not worth becoming one for a single constant.
const uniqueViolation = "23505"

// isUniqueViolation reports whether err is that breach. Used where a constraint
// is the real arbiter of a rule and the pre-check above it can be raced — so
// losing that race is a 409 like any other, not a 500.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// setKind maps the is_assistance flag the session queries derive onto the wire
// enum, so the mapping lives in one place rather than at each call site.
func setKind(isAssistance bool) string {
	if isAssistance {
		return exerciseKindAssistance
	}
	return exerciseKindMain
}

func internalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "internal", "internal server error")
}

// idParam reads a positive int32 path parameter (all IDs are SERIAL/int32).
func idParam(r *http.Request, key string) (int32, bool) {
	v, err := strconv.ParseInt(chi.URLParam(r, key), 10, 32)
	if err != nil || v <= 0 {
		return 0, false
	}
	return int32(v), true
}
