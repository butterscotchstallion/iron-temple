// Package api implements the Iron Temple HTTP API defined by openapi.yaml.
// Handlers read and write the DTOs in dto.go; persistence goes through the
// sqlc-generated store, and next-session weights come from the progression
// engine.
package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gorilla/websocket"

	"gitea.homelab/gitadmin/iron-temple/api/internal/auth"
	"gitea.homelab/gitadmin/iron-temple/api/internal/live"
	"gitea.homelab/gitadmin/iron-temple/api/internal/metrics"
	"gitea.homelab/gitadmin/iron-temple/api/internal/racked"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Server holds the dependencies shared by every handler.
type Server struct {
	pool        *pgxpool.Pool
	q           *store.Queries
	version     string
	environment string
	// hasher is the interface, not the concrete type, so the test binary can
	// swap in the same implementation at a cheap work factor — see
	// export_test.go. Production never sets it to anything but the zero
	// PBKDF2Hasher NewServer installs below.
	hasher auth.Hasher
	// logins brakes password guessing. In-process state, so it is per-replica —
	// see the type's doc for why that is the right trade here.
	logins *auth.RateLimiter
	// comments brakes the one write a signed-in account can repeat freely. Its
	// own limiter rather than a shared one because the two count different
	// things — logins count failures, this counts posts (see Consume) — and
	// because a key here is a user id where a key there is an address and a
	// username, so one map would hold two namespaces and hope they never meet.
	comments *auth.RateLimiter
	// reportLoc is the zone the Racked recap reads clock times in. Dates are
	// stored as dates and need no zone; session start times are instants, and
	// "you are an early riser" is a claim about local mornings.
	reportLoc *time.Location
	// mailer delivers the Racked recap. Nil disables the reporter entirely,
	// which is how tests and local development avoid sending real mail — the
	// integration suite drives sendDueReports directly instead.
	mailer *racked.Mailer
	// activity is the generated-training runner's state. Always present: it holds
	// a mutex and a flag, and the flag is false until an admin turns it on, so
	// there is nothing to configure and nothing to nil-check.
	activity *activityRunner
	// metrics counts what the API serves. Always present — a nil check at every
	// observation point is a worse trade than a registry nobody scrapes, and
	// the exposition page is only reachable from the address main.go binds it
	// to, not from this router.
	metrics *metrics.Registry
	// live is every open WebSocket on this process. Always present, like
	// activity and metrics above and for the same reason: it holds a map and a
	// mutex, it starts nothing, and a nil check at every publish site is a
	// worse trade than a hub nobody connects to.
	live *live.Hub
}

// NewServer builds a Server over a pgx connection pool. version and environment
// are surfaced by the health endpoint (and the UI header bar); environment also
// decides whether session cookies are marked Secure.
func NewServer(pool *pgxpool.Pool, version, environment string) *Server {
	s := &Server{
		pool:        pool,
		q:           store.New(pool),
		version:     version,
		environment: environment,
		hasher:      auth.PBKDF2Hasher{},
		logins:      auth.NewRateLimiter(auth.DefaultAttempts, auth.DefaultWindow),
		comments:    auth.NewRateLimiter(maxCommentsPerWindow, commentWindow),
		reportLoc:   time.UTC,
		activity:    &activityRunner{},
		metrics:     metrics.New(version, environment),
		live:        live.New(live.Options{CheckOrigin: liveOrigin}),
	}
	// Pool saturation is the failure this deployment is most likely to hit —
	// one small pool, a reporter and a sweeper sharing it with request traffic
	// — so it is wired up here rather than left to the caller to remember. The
	// closure is evaluated per scrape, not now.
	if pool != nil {
		s.metrics.SetPoolSource(func() metrics.PoolStats {
			stat := pool.Stat()
			return metrics.PoolStats{
				Acquired: stat.AcquiredConns(),
				Idle:     stat.IdleConns(),
				Total:    stat.TotalConns(),
				Max:      stat.MaxConns(),
			}
		})
	}
	// A long-lived connection is a gauge, not a request — see observe() for why
	// the request middleware skips a socket, and this is what replaces the
	// accounting it skips. Evaluated per scrape, like the pool above.
	s.metrics.SetLiveSource(func() metrics.LiveStats {
		stat := s.live.Stats()
		return metrics.LiveStats{
			Connections: stat.Connections,
			Sent:        stat.Sent,
			Dropped:     stat.Dropped,
		}
	})
	return s
}

// Metrics returns the registry this server records into, so main.go can serve
// its exposition page. Deliberately not mounted on the API router — see
// cmd/server for where it is bound and why that is a different listener.
func (s *Server) Metrics() *metrics.Registry { return s.metrics }

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
	// Outside Recoverer on purpose. Middleware runs in registration order, so
	// this wraps it: a handler that panics is turned into a 500 by Recoverer
	// *inside* the wrapped writer, and the observation reads that 500. The
	// other way round, the panic would unwind past this deferred observation
	// before any status had been written, and the request that took the
	// process closest to failing would be recorded as a success.
	r.Use(s.observe)
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
			// Below requireUser, so a 401 is never given an ETag and cached as
			// though it were an answer.
			r.Use(jsonETag)

			r.Route("/me", func(r chi.Router) {
				// The two endpoints an account with a pending password change
				// can still reach, and the only two: the UI learns that it must
				// change the password by reading /me, and the PUT below is the
				// way out. Gating either would make the state inescapable.
				r.Get("/", s.getMe)
				r.Put("/password", s.changePassword)

				r.Group(func(r chi.Router) {
					r.Use(s.blockUntilPasswordChanged)
					r.Patch("/", s.updateMe)
					r.Post("/avatar", s.uploadAvatar)
					r.Delete("/avatar", s.deleteAvatar)
					r.Get("/baselines", s.listBaselines)
					r.Put("/baselines/{exerciseId}", s.setBaseline)
					r.Delete("/baselines/{exerciseId}", s.clearBaseline)

					// Whose achievements the caller hears about. Here rather
					// than under /lifters BECAUSE it writes: that subtree is
					// read-only by construction and must stay that way, and
					// the resource being written is the caller's own list.
					// See the header in follows.go.
					r.Post("/following/{lifterId}", s.followLifter)
					r.Delete("/following/{lifterId}", s.unfollowLifter)

					// Leaving the House you are in, and here for the same
					// reason the two above are: the subject is the caller. No
					// id to give either, since a lifter is in at most one
					// House — which is why this is not under /houses/{houseId}.
					r.Delete("/house", s.leaveHouse)
				})
			})

			// Everything else needs a password this account's own owner chose.
			// Grouped rather than applied route by route for the same reason
			// requireUser is: a handler added later inherits the gate by sitting
			// here, and escaping it takes a deliberate move up to the two
			// exemptions above.
			r.Group(func(r chi.Router) {
				r.Use(s.blockUntilPasswordChanged)

				// The install's account management. requireAdmin is mounted on
				// the subtree rather than on each handler, so a third admin
				// endpoint cannot be added unguarded.
				r.Route("/admin", func(r chi.Router) {
					r.Use(s.requireAdmin)
					r.Get("/users", s.listUsers)
					r.Post("/users", s.createUser)

					// Generated training activity, for an install that needs
					// some to look at. Inside this subtree on purpose: these are
					// the most powerful handlers in the app — one fabricates
					// history and one deletes accounts — and sitting here is what
					// makes them admin-only without anybody having to remember,
					// exactly as the note above intends.
					r.Get("/activity", s.getActivityStatus)
					r.Get("/activity/schedule", s.getActivitySchedule)
					r.Put("/activity/schedule", s.putActivitySchedule)
					r.Post("/activity/backfill", s.postActivityBackfill)
					r.Post("/activity/start", s.postActivityStart)
					r.Post("/activity/stop", s.postActivityStop)
					r.Delete("/activity", s.deleteActivity)
				})

				// The install's recent activity. Not under /lifters because no id
				// in its path names a person — see feed.go.
				r.Get("/feed", s.getFeed)
				// How the lifters compare. Also top-level, and for the same
				// reason: the subject is the install, not a lifter.
				r.Get("/leaderboard", s.getLeaderboard)
				// What can be earned, and who is currently wearing it.
				// Top-level for the leaderboard's reason — the subject is the
				// install — and the read every client makes in order to draw a
				// crown beside somebody else's name, which is why it answers
				// for everybody at once rather than per lifter.
				r.Get("/achievements", s.getAchievements)

				// Houses. The collection read is top-level for the same reason
				// /achievements is — the subject is the install, and it is the
				// read every client makes in order to draw a sigil beside
				// somebody else's name, so it answers for everybody at once.
				//
				// The writes are all here rather than on a path naming a
				// person, which is the arrangement lifters.go asks for: the
				// caller is the authenticated user, and the ids in these paths
				// are a House and a request, never a lifter. The one write
				// whose subject IS the caller — leaving — is on /me/house.
				r.Route("/houses", func(r chi.Router) {
					r.Get("/", s.listHouses)
					r.Post("/", s.createHouse)
					r.Get("/{houseId}", s.getHouse)
					r.Patch("/{houseId}", s.updateHouse)
					r.Post("/{houseId}/requests", s.requestToJoinHouse)
					r.Delete("/{houseId}/requests/{requestId}", s.withdrawHouseRequest)
					// Owner-only, checked in the handlers rather than by a
					// middleware on a subtree: unlike requireAdmin, the answer
					// depends on which House is in the path, so there is nothing
					// a mount point could decide on its own.
					r.Post("/{houseId}/requests/{requestId}/approve", s.approveHouseRequest)
					r.Post("/{houseId}/requests/{requestId}/decline", s.declineHouseRequest)
				})

				// What happened to the caller. Top-level like the two above,
				// but for the opposite reason: the subject IS a person, and it
				// is always the one holding the session cookie. There is no id
				// in the path because there is nobody else these could be for
				// — see notifications.go for why there is no POST either.
				r.Get("/notifications", s.listNotifications)
				r.Delete("/notifications", s.clearNotifications)
				r.Post("/notifications/read", s.markNotificationsRead)
				// One row, for a lifter who followed a notification through to
				// what it was about. Registered after the collection's own
				// /read above, which chi would not confuse in any case — the
				// two patterns differ in their first segment.
				r.Post("/notifications/{notificationId}/read", s.markNotificationRead)
				// The same row, unfolded. Beside the POST above because they
				// resolve a group from an id the same way — one to stamp every
				// member, one to read them — and a surface that details a
				// folded row needs both.
				r.Get("/notifications/{notificationId}/members", s.getNotificationGroupMembers)

				// One lifter reading another. Every route is a GET, and that is
				// load-bearing rather than incidental: the id in these paths
				// names a person whose history is being read, so a write
				// handler here would be one lifter editing another's training.
				// See the header comment in lifters.go.
				r.Route("/lifters", func(r chi.Router) {
					r.Get("/", s.listLifters)
					r.Get("/{lifterId}", s.getLifter)
					r.Get("/{lifterId}/racked", s.getLifterRacked)
					r.Get("/{lifterId}/sessions", s.getLifterSessions)
					r.Get("/{lifterId}/sessions/{sessionId}/recap", s.getLifterSessionRecap)
					// One lifter's achievements, current and past. Here rather
					// than beside /achievements because this one's subject IS a
					// person, and there is no /me variant — a lifter's own
					// achievements are the same public facts as anybody else's.
					r.Get("/{lifterId}/achievements", s.getLifterAchievements)
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
					// A program of the caller's own. Every write below scopes on
					// created_by_user_id, which is NULL on the seeded programs —
					// so the install's catalogue is uneditable by construction
					// rather than by a rule each handler remembers.
					r.Post("/", s.createProgram)
					r.Patch("/{programId}", s.updateProgram)
					// Archive is a sub-resource because this cannot delete:
					// sessions reference a program's days and the key RESTRICTs,
					// so retiring is the only removal on offer and a DELETE here
					// would be a lie about what happened.
					r.Post("/{programId}/archive", s.archiveProgram)
					r.Delete("/{programId}/archive", s.unarchiveProgram)
					r.Get("/{programId}/days/{dayId}/next-session", s.previewNextSession)
					r.Get("/{programId}/next-sessions", s.previewNextSessions)
					// Days and the lifts on them. Owner-only, except the
					// weekday — see updateProgramDay for why that one field
					// stays editable on the seeded programs too.
					r.Post("/{programId}/days", s.addProgramDay)
					r.Patch("/{programId}/days/{dayId}", s.updateProgramDay)
					r.Delete("/{programId}/days/{dayId}", s.removeProgramDay)
					r.Put("/{programId}/days/order", s.reorderProgramDays)
					r.Post("/{programId}/days/{dayId}/exercises", s.addPrescription)
					r.Patch("/{programId}/days/{dayId}/exercises/{prescriptionId}",
						s.updatePrescription)
					r.Delete("/{programId}/days/{dayId}/exercises/{prescriptionId}",
						s.removePrescription)
					r.Put("/{programId}/days/{dayId}/exercises/order", s.reorderPrescriptions)
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
					r.Get("/{sessionId}/recap", s.getSessionRecap)
					// Applause and conversation. These DO write, unlike
					// everything under /lifters, and it is safe for the reason
					// recognition.go gives: the id in the path names a session,
					// and the author is always the authenticated caller, so
					// there is no path-supplied user id to get wrong.
					r.Get("/{sessionId}/reactions", s.listSessionReactions)
					r.Post("/{sessionId}/reactions", s.addSessionReaction)
					r.Delete("/{sessionId}/reactions", s.removeSessionReaction)
					r.Get("/{sessionId}/comments", s.listSessionComments)
					r.Post("/{sessionId}/comments", s.addSessionComment)
					r.Delete("/{sessionId}/comments/{commentId}", s.deleteSessionComment)
					r.Post("/{sessionId}/assistance", s.addSessionAssistance)
					r.Post("/{sessionId}/sets", s.addSessionSet)
					r.Patch("/{sessionId}/sets/{setId}", s.updateSessionSet)
					r.Delete("/{sessionId}/sets/{setId}", s.removeSessionSet)
				})
			})
		})

		// The export sits outside the group above because it must not be
		// ETagged, and requireUser is therefore spelled out here rather than
		// inherited. Two reasons it is excluded, one of which is fatal to the
		// idea: the document embeds the instant it was produced, so its bytes
		// differ on every request and a validator computed from them can never
		// match. The other is that tagging means buffering the whole response
		// to hash it, and this is the one endpoint whose response grows without
		// bound as the training history does.
		//
		// The forced-password-change gate is spelled out alongside it for the
		// same reason: this route inherits nothing, so anything the group above
		// applies has to be repeated here or it simply does not apply. Handing
		// an account's entire training history to a session still holding a
		// password somebody else chose is exactly what that gate is for.
		r.With(s.requireUser, s.blockUntilPasswordChanged).Get("/me/export", s.exportAccount)

		// The live socket, outside the group for a harder version of the
		// export's reason: jsonETag replaces the writer with a recorder that is
		// not an http.Hijacker, and an upgrade needs the raw connection — so
		// inside the group this route could not work at all. Both gates are
		// repeated because this route inherits nothing. See internal/api/live.go
		// and docs/live-socket.md.
		r.With(s.requireUser, s.blockUntilPasswordChanged).Get("/live", s.serveLive)
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

// jsonETag adds conditional-GET support to JSON reads.
//
// The UI keeps a stale-while-revalidate cache: it paints last-known data
// immediately and refetches behind it on every mount. That makes revalidation
// the app's most common request by some margin, and almost all of it returns
// exactly what the caller already has — a full session list re-serialized and
// re-sent to say "unchanged". An ETag turns that into a 304 with no body.
//
// Applied as middleware rather than inside writeJSON so it covers every JSON
// read at once, including handlers added later, and so it can see the request
// it is answering — writeJSON only ever gets the writer.
//
// Only plain GETs of 200 JSON are touched. A 4xx is left alone (caching an
// error would be worse than re-sending it), and so is any handler that already
// set an ETag of its own: getUserAvatar serves binary with a hash it computes
// from the stored image, and buffering that through here would be both wasteful
// and wrong.
func jsonETag(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}

		rec := &etagRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		body := rec.body.Bytes()
		header := w.Header()
		eligible := rec.status == http.StatusOK &&
			header.Get("ETag") == "" &&
			strings.HasPrefix(header.Get("Content-Type"), "application/json")
		if !eligible {
			w.WriteHeader(rec.status)
			_, _ = w.Write(body)
			return
		}

		// Weak comparison is what If-None-Match uses, and the tag is a digest of
		// the exact bytes about to be sent, so a strong tag would claim more than
		// it can: the same content re-serialized is the same response as far as
		// this endpoint is concerned.
		sum := sha256.Sum256(body)
		etag := fmt.Sprintf(`W/"%s"`, hex.EncodeToString(sum[:16]))
		header.Set("ETag", etag)
		// Must-revalidate rather than a max-age: the client is already asking on
		// every mount, and what we are saving is the body, not the round trip.
		// A max-age would let it show stale data without asking, which is the
		// cache's own job and its own rules.
		header.Set("Cache-Control", "private, max-age=0, must-revalidate")

		if matchesETag(r.Header.Get("If-None-Match"), etag) {
			// A 304 carries no body, and Content-Length would be a lie about one.
			header.Del("Content-Length")
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.WriteHeader(rec.status)
		_, _ = w.Write(body)
	})
}

// observe records every request in the metrics registry.
//
// The `route` label is chi's route PATTERN, not the path: `/sessions/{sessionId}`
// rather than `/sessions/412`. Labelling by path would mint a time series per
// session id, and the series count would then grow with the training history —
// the classic way a metrics endpoint becomes the most expensive thing in a
// deployment. An unrouted request has no pattern and is folded into a single
// "unmatched" series by the registry, for the same reason.
// A WEBSOCKET IS NOT A REQUEST, and this middleware has to say so.
//
// The in-flight gauge is held for the handler's whole lifetime and one duration
// sample is recorded when it returns. For an ordinary request that is exactly
// right; for a socket a lifter leaves open all day it is two lies — the gauge
// would count an idle connection as work in flight, and the histogram would get
// a 43,000-second sample into a bucket set whose largest finite bound is ten.
// Both would then be wrong for every other route that shares the series.
//
// So an upgrade that actually BECAME a socket is skipped, and one that did not
// is counted as usual. rec.Status() == 0 is the exact discriminator: a
// successful upgrade hijacks the connection and never writes a status through
// the wrapper, while a 401 from requireUser, a 403 from the origin check and
// gorilla's own 400 all do. That keeps handshake FAILURES — the part worth
// alerting on — fully observable.
//
// What replaces the skipped accounting is a connection gauge the hub reports
// per scrape; see NewServer.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		upgrade := websocket.IsWebSocketUpgrade(r)

		// The gauge is only taken for something that can give it back.
		var done func(method, route string, status int, d time.Duration)
		if !upgrade {
			done = s.metrics.RequestStarted()
		}

		// chi's wrapper rather than a local one: it preserves Flush and Hijack
		// through the chain, which a bare struct embedding ResponseWriter would
		// silently drop.
		rec := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		defer func() {
			status := rec.Status()
			if upgrade && status == 0 {
				// Hijacked: it became a socket. Nothing to record.
				return
			}
			// A handler that returns without ever writing a header has served
			// a 200, which is what net/http will send.
			if status == 0 {
				status = http.StatusOK
			}
			// RoutePattern is only populated once routing has happened, which
			// is why this is read here and not before next.
			route := chi.RouteContext(r.Context()).RoutePattern()
			if done != nil {
				done(r.Method, route, status, time.Since(start))
				return
			}
			// A failed upgrade, which never took the gauge.
			s.metrics.ObserveRequest(r.Method, route, status, time.Since(start))
		}()

		next.ServeHTTP(rec, r)
	})
}

// matchesETag reports whether an If-None-Match header names this tag. The header
// is a comma-separated list and may be "*"; entries are compared weakly, so
// W/"x" and "x" are the same validator.
func matchesETag(header, etag string) bool {
	if header == "" {
		return false
	}
	if strings.TrimSpace(header) == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimPrefix(strings.TrimSpace(candidate), "W/") == want {
			return true
		}
	}
	return false
}

// etagRecorder buffers a response so its body can be hashed before any of it is
// committed. Responses here are a session or a list of them — small enough that
// holding one in memory costs less than re-sending it would.
type etagRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (rec *etagRecorder) WriteHeader(status int) { rec.status = status }

func (rec *etagRecorder) Write(p []byte) (int, error) { return rec.body.Write(p) }

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

// pageParams reads the `limit` and `offset` query parameters shared by every
// paged list, writing the 400 itself when either is out of range. ok=false means
// the caller should stop.
//
// ParseInt with bitSize 32 is what bounds these to int32, so the conversions
// cannot overflow — an offset of 3000000000 is a 400 rather than a value that
// wraps negative on the way into the query. (It also clears gosec G109/G115.)
// That bound is the reason this is one function and not a convention: it was
// originally missing from offset, and a second copy of the parsing is a second
// place for it to go missing again.
func pageParams(w http.ResponseWriter, r *http.Request) (limit, offset int32, ok bool) {
	query := r.URL.Query()

	limit = int32(defaultLimit)
	if v := query.Get("limit"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 1 || n > maxLimit {
			badRequest(w, "limit must be between 1 and 100")
			return 0, 0, false
		}
		limit = int32(n)
	}

	if v := query.Get("offset"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil || n < 0 {
			badRequest(w, "offset must be >= 0")
			return 0, 0, false
		}
		offset = int32(n)
	}
	return limit, offset, true
}
