// Command server applies migrations, then serves the Iron Temple HTTP API.
// Configuration is via environment:
//
//	DATABASE_URL  (required)  Postgres DSN, connected as the iron_temple tenant.
//	PORT          (default 8080)
//	CORS_ORIGIN   (optional)  comma-separated UI origins; empty allows any.
//	REPORT_TZ     (default UTC) IANA zone the Racked recap reads clock times in.
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
	"syscall"
	"time"

	// The release image is FROM scratch and carries no zone database, so
	// REPORT_TZ would silently degrade to UTC on every deployment. Embedding
	// tzdata costs ~450 KB and makes the setting mean what it says.
	_ "time/tzdata"

	_ "github.com/lib/pq"

	"github.com/jackc/pgx/v5/pgxpool"

	appdb "gitea.homelab/gitadmin/iron-temple/api/db"
	"gitea.homelab/gitadmin/iron-temple/api/internal/api"
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

	// Expired login sessions are already ignored by every query; this reaps the
	// dead rows so the table doesn't grow for the life of the deployment.
	sweepCtx, stopSweeper := context.WithCancel(ctx)
	defer stopSweeper()
	apiSrv.StartSessionSweeper(sweepCtx, time.Hour)

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

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		return err
	case <-stop:
		log.Println("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
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

// migrate applies the embedded schema via database/sql (lib/pq), reusing the
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
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	return appdb.Migrate(sqlDB)
}
