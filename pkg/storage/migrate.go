package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
)

// RunMigrations reads *.sql files from dir (sorted by name) and executes them in order.
// Files should be idempotent — use IF NOT EXISTS etc.
func RunMigrations(db *sql.DB, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("storage: read migrations dir %s: %w", dir, err)
	}

	files := make([]string, 0)
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, f := range files {
		path := filepath.Join(dir, f)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("storage: read %s: %w", path, err)
		}
		if _, err := db.ExecContext(context.Background(), string(content)); err != nil {
			return fmt.Errorf("storage: exec %s: %w", path, err)
		}
		log.Printf("migration applied: %s", f)
	}
	return nil
}
