// migrate — apply / roll back / drop schema migrations against the database
// pointed to by DATABASE_URL (read from .env or environment).
//
// Usage:
//   go run ./cmd/migrate up        — apply all pending migrations
//   go run ./cmd/migrate down      — roll back the most recent migration
//   go run ./cmd/migrate reset     — drop everything and re-apply (DANGEROUS)
//   go run ./cmd/migrate version   — print current migration version
//
// Works against any Postgres reachable by the URL — local Docker, AWS RDS, etc.
// No external `migrate` binary needed.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

func main() {
	migrationsPath := flag.String("path", "../migrations", "filesystem path to migrations dir (relative to backend/)")
	flag.Parse()

	cmd := "up"
	if flag.NArg() > 0 {
		cmd = flag.Arg(0)
	}

	// Load .env from the repo root (../.env) so cloud users only set DATABASE_URL once.
	_ = godotenv.Load("../.env")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required (set in .env or environment)")
	}

	src := "file://" + *migrationsPath
	m, err := migrate.New(src, dbURL)
	if err != nil {
		log.Fatalf("migrate init: %v", err)
	}
	defer m.Close()

	switch cmd {
	case "up":
		err = m.Up()
	case "down":
		err = m.Steps(-1)
	case "reset":
		// drop + re-apply
		if dropErr := m.Drop(); dropErr != nil && !errors.Is(dropErr, migrate.ErrNoChange) {
			log.Fatalf("migrate drop: %v", dropErr)
		}
		// `Drop()` invalidates the instance — re-create it.
		m2, e := migrate.New(src, dbURL)
		if e != nil {
			log.Fatalf("migrate re-init: %v", e)
		}
		defer m2.Close()
		err = m2.Up()
	case "version":
		v, dirty, vErr := m.Version()
		if errors.Is(vErr, migrate.ErrNilVersion) {
			fmt.Println("no version (no migrations applied)")
			return
		}
		if vErr != nil {
			log.Fatal(vErr)
		}
		fmt.Printf("version=%d dirty=%v\n", v, dirty)
		return
	default:
		log.Fatalf("unknown command %q (use up | down | reset | version)", cmd)
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("migrate %s: %v", cmd, err)
	}
	fmt.Printf("✓ migrate %s\n", cmd)
}
