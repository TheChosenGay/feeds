package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// NewMySQLPool opens a *sql.DB backed by go-sql-driver/mysql. Caller is responsible for defer db.Close().
func NewMySQLPool(ctx context.Context, dsn string, maxConns int) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open mysql: %w", err)
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns / 2)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: ping mysql: %w", err)
	}
	return db, nil
}

// MySQLHealth reports whether the pool is reachable.
func MySQLHealth(db *sql.DB) error {
	return db.PingContext(context.Background())
}
