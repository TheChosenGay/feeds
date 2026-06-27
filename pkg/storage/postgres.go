package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for database/sql
	"github.com/XSAM/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// NewPostgresPool opens a *sql.DB backed by pgx. Caller is responsible for defer db.Close().
func NewPostgresPool(ctx context.Context, dsn string, maxConns int) (*sql.DB, error) {
	db, err := otelsql.Open("pgx", dsn,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: open postgres: %w", err)
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns / 2)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: ping postgres: %w", err)
	}
	return db, nil
}

// PostgresHealth reports whether the pool is reachable.
func PostgresHealth(db *sql.DB) error {
	return db.PingContext(context.Background())
}
