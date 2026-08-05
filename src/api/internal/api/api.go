// Package api implements the Iron Temple HTTP API defined by openapi.yaml.
// Handlers read and write the DTOs in dto.go; persistence goes through the
// sqlc-generated store, and next-session weights come from the progression
// engine.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Server holds the dependencies shared by every handler.
type Server struct {
	pool *pgxpool.Pool
	q    *store.Queries
}

// NewServer builds a Server over a pgx connection pool.
func NewServer(pool *pgxpool.Pool) *Server {
	return &Server{pool: pool, q: store.New(pool)}
}

// Router returns the fully-wired HTTP handler. corsOrigin is a comma-separated
// allowlist of UI origins; empty means allow any origin (dev convenience).
func (s *Server) Router(corsOrigin string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: corsOrigins(corsOrigin),
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders: []string{"Accept", "Content-Type"},
		MaxAge:         300,
	}))

	// All paths live under the OpenAPI server base path.
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.getHealth)
		r.Get("/exercises", s.listExercises)

		r.Route("/programs", func(r chi.Router) {
			r.Get("/", s.listPrograms)
			r.Get("/{programId}", s.getProgram)
			r.Get("/{programId}/days/{dayId}/next-session", s.previewNextSession)
		})

		r.Route("/sessions", func(r chi.Router) {
			r.Get("/", s.listSessions)
			r.Post("/", s.createSession)
			r.Get("/{sessionId}", s.getSession)
			r.Patch("/{sessionId}", s.updateSession)
			r.Delete("/{sessionId}", s.deleteSession)
			r.Patch("/{sessionId}/sets/{setId}", s.updateSessionSet)
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
