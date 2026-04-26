package db

import (
	"errors"
	"log"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// MigrateUp applies all pending migrations from `migrationsDir` (a filesystem
// path relative to the cwd at runtime). Idempotent: no-op if the DB is already
// up to date. Used in main.go so freshly-deployed services self-heal their
// schema without needing a Render pre-deploy hook (which the free plan blocks).
func MigrateUp(dbURL, migrationsDir string) error {
	m, err := migrate.New("file://"+migrationsDir, dbURL)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	v, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		log.Printf("migrate: warning reading version: %v", err)
		return nil
	}
	log.Printf("migrate: schema at version=%d dirty=%v", v, dirty)
	return nil
}
