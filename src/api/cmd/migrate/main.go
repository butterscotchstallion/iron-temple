// Command migrate applies the embedded schema migrations to the database named
// by DATABASE_URL. Intended to run as an init step before the API starts (and
// usable from the deploy/ Job once the app image exists).
package main

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"

	"gitea.homelab/gitadmin/iron-temple/api/db"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	if err := db.Migrate(sqlDB); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("migrations applied")
}
