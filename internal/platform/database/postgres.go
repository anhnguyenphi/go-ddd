// Package database opens and configures the application's SQL connection pool.
//
// No driver is imported here on purpose: the skeleton runs fully in-memory by
// default. To enable Postgres, add a driver (e.g. github.com/jackc/pgx/v5/stdlib
// or github.com/lib/pq) with a blank import in cmd/*/main.go, set DATABASE_URL,
// and the bootstrap package will switch to the SQL-backed wiring.
package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/example/myapp/internal/platform/config"
)

// Open returns a ready connection pool, verified with a Ping.
func Open(ctx context.Context, cfg config.DatabaseConfig) (*sql.DB, error) {
	// "pgx" is the conventional name registered by jackc/pgx's stdlib package.
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
