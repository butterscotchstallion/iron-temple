// Command server applies migrations, then serves the Iron Temple HTTP API.
// Configuration is via environment:
//
//	DATABASE_URL  (required)  Postgres DSN, connected as the iron_temple tenant.
//	PORT          (default 8080)
//	CORS_ORIGIN   (optional)  comma-separated UI origins; empty allows any.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           api.NewServer(pool, version, environment).Router(os.Getenv("CORS_ORIGIN")),
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
