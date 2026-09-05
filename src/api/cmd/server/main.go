// Command server applies migrations, then serves the Iron Temple HTTP API.
// Configuration is via environment:
//
//	DATABASE_URL  (required)  Postgres DSN, connected as the iron_temple tenant.
//	PORT          (default 8080)
//	METRICS_PORT  (default 9090) Prometheus exposition, on its own listener.
//	              "off" disables it.
//	CORS_ORIGIN   (optional)  comma-separated UI origins; empty allows any.
//	REPORT_TZ     (default UTC) IANA zone the Racked recap reads clock times in.
//	REPORT_MAIL   (default on outside development) "off" disables the Racked
//	              recap emails entirely.
//	MAIL_RELAY    (default the in-cluster relay) endpoint the recap is POSTed to.
//	REPORT_MAIL_TO (default alerts@homelab.local) recap recipient.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	// The release image is FROM scratch and carries no zone database, so
	// REPORT_TZ would silently degrade to UTC on every deployment. Embedding
	// tzdata costs ~450 KB and makes the setting mean what it says.
	_ "time/tzdata"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/jackc/pgx/v5/pgxpool"

	appdb "gitea.homelab/gitadmin/iron-temple/api/db"
	"gitea.homelab/gitadmin/iron-temple/api/internal/api"
	"gitea.homelab/gitadmin/iron-temple/api/internal/racked"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// version is set at build time via -ldflags "-X main.version=vX.Y.Z"; "dev" for
// local builds. Surfaced (with ENVIRONMENT) by the health endpoint + UI footer.
var version = "dev"

// resolvedVersion returns the release version stamped via -ldflags (main.version),
// or for local/dev builds falls back to the embedded VCS revision — "dev-<sha>"
// (plus "-dirty" for uncommitted changes). Degrades to plain "dev" if the build
// carries no VCS info.
func resolvedVersion() string {
	if version != "dev" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 7 {
				rev = rev[:7]
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev == "" {
		return version
	}
	return "dev-" + rev + dirty
}

func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		environment = "development"
	}

	if err := migrateWithRetry(dsn); err != nil {
		return err
	}
	log.Println("migrations applied")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	apiSrv := api.NewServer(pool, resolvedVersion(), environment)
	apiSrv.SetReportLocation(reportLocation())
	if mailer := reportMailer(environment); mailer != nil {
		apiSrv.SetMailer(mailer)
	}

	// Expired login sessions are already ignored by every query; this reaps the
	// dead rows so the table doesn't grow for the life of the deployment.
	sweepCtx, stopSweeper := context.WithCancel(ctx)
	defer stopSweeper()
	apiSrv.StartSessionSweeper(sweepCtx, time.Hour)

	// The Racked recap. Hourly rather than daily because the tick is a question
	// ("is a period outstanding?"), not an alarm — see StartRackedReporter.
	apiSrv.StartRackedReporter(sweepCtx, time.Hour)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           apiSrv.Router(os.Getenv("CORS_ORIGIN")),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Serve until an interrupt, then drain in-flight requests.
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	metricsSrv := metricsServer(apiSrv)
	if metricsSrv != nil {
		go func() {
			log.Printf("metrics on %s/metrics", metricsSrv.Addr)
			// Deliberately not reported on serveErr. Prometheus not being able
			// to scrape is a monitoring outage; the API refusing to start over
			// it would turn that into a training outage.
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("metrics listener stopped: %v", err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		return err
	case <-stop:
		log.Println("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if metricsSrv != nil {
			// Closed first and without waiting on it: a scrape is a read of a
			// few kilobytes, and nothing is lost by cutting one short. The API's
			// own drain is the one worth the ten seconds.
			_ = metricsSrv.Shutdown(shutdownCtx)
		}
		return srv.Shutdown(shutdownCtx)
	}
}

// metricsServer builds the listener the Prometheus exposition page is served
// on, or nil when METRICS_PORT is "off".
//
// A SEPARATE listener from the API, which is the whole point. Traefik routes
// the public hostname at the API's port, so anything mounted on that port is
// reachable from outside the cluster — and this page names every route, counts
// every error and reports the traffic volume of a household. On its own port it
// is reachable from inside the cluster only, where the scraper lives, without
// depending on getting an ingress exclusion right.
func metricsServer(apiSrv *api.Server) *http.Server {
	port := strings.TrimSpace(os.Getenv("METRICS_PORT"))
	if port == "" {
		port = "9090"
	}
	if strings.EqualFold(port, "off") {
		return nil
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", apiSrv.Metrics().Handler())
	return &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// reportLocation resolves REPORT_TZ, falling back to UTC. A zone the runtime
// cannot load is logged and ignored rather than fatal: the recap saying "early
// bird" an hour off is a smaller problem than the API refusing to boot, and
// the scratch image carries no tzdata unless one is built in.
func reportLocation() *time.Location {
	name := os.Getenv("REPORT_TZ")
	if name == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Printf("REPORT_TZ %q not loadable, using UTC: %v", name, err)
		return time.UTC
	}
	return loc
}

// reportMailer builds the Racked recap's mail client, or nil to disable recaps.
//
// Off by default in development, because `make dev` against a real DATABASE_URL
// would otherwise mail a recap the first time it ran in a new month — from a
// laptop, to the shared alerts address. REPORT_MAIL=on opts a local run in;
// REPORT_MAIL=off opts a deployment out.
func reportMailer(environment string) *racked.Mailer {
	setting := strings.ToLower(strings.TrimSpace(os.Getenv("REPORT_MAIL")))
	enabled := environment != "development"
	switch setting {
	case "on", "true", "1":
		enabled = true
	case "off", "false", "0":
		enabled = false
	}
	if !enabled {
		return nil
	}
	return racked.NewMailer(os.Getenv("MAIL_RELAY"), os.Getenv("REPORT_MAIL_TO"))
}

// migrate applies the embedded schema via database/sql (pgx stdlib), reusing the
// same migrator as cmd/migrate so startup and the standalone tool stay in sync.
// migrateWithRetry runs the boot migration, retrying on transient failures. A new
// pod's egress NetworkPolicy is applied a moment after start (kube-router race,
// k8s-networking-gotchas §5), so the first DB dial can be refused — retry with
// backoff (~36s) instead of crash-looping.
func migrateWithRetry(dsn string) error {
	var err error
	for i := 1; i <= 12; i++ {
		if err = migrate(dsn); err == nil {
			return nil
		}
		log.Printf("db not ready (attempt %d/12): %v", i, err)
		time.Sleep(3 * time.Second)
	}
	return err
}

func migrate(dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	return appdb.Migrate(sqlDB)
}
