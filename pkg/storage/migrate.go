package storage

import (
	"fmt"
	"io/fs"
	"log"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// RunMigrations applies up migrations from a directory on disk.
func RunMigrations(dsn, dir string) error {
	m, err := migrate.New("file://"+dir, dsn)
	if err != nil {
		return fmt.Errorf("storage: init migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("storage: run migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	log.Printf("migrations up to date: version=%d dirty=%v", version, dirty)
	return nil
}

// RunMigrationsFS applies up migrations from an embedded filesystem (go:embed).
// fsys is the embedded FS; sub is the subdirectory within it (use "." for root).
func RunMigrationsFS(dsn string, fsys fs.FS) error {
	src, err := iofs.New(fsys, ".")
	if err != nil {
		return fmt.Errorf("storage: init iofs: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("storage: init migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("storage: run migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	log.Printf("migrations up to date: version=%d dirty=%v", version, dirty)
	return nil
}
