package db

import "embed"

// migrationsFS holds the SQL migrations so they travel with the binary and the
// test suite — no migrations directory needs to exist at runtime.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS
