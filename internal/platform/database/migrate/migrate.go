// Package migrate discovers and applies NNNN_name.{up,down}.sql migrations,
// tracking progress in a schema_migrations table. Each file runs in its own
// transaction. It is shared by cmd/migrate and by the infra integration tests,
// so both apply the schema the same way.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Direction is "up" or "down".
type Direction string

const (
	Up   Direction = "up"
	Down Direction = "down"
)

var filenameRe = regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)

// Migration groups the up/down pair discovered for one version.
type Migration struct {
	Version  string
	Name     string
	UpPath   string
	DownPath string
}

// Path returns the file for the given direction ("" if missing).
func (m Migration) Path(d Direction) string {
	if d == Up {
		return m.UpPath
	}
	return m.DownPath
}

// Open connects with the simple query protocol, which (unlike the default
// extended protocol) lets a single Exec run a file with several statements.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	connConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	db := stdlib.OpenDB(*connConfig)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// Load discovers and pairs up *.up.sql / *.down.sql files in dir, sorted by
// version ascending.
func Load(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	byVersion := map[string]*Migration{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		match := filenameRe.FindStringSubmatch(e.Name())
		if match == nil {
			continue
		}
		version, name, kind := match[1], match[2], match[3]

		m, ok := byVersion[version]
		if !ok {
			m = &Migration{Version: version, Name: name}
			byVersion[version] = m
		}
		path := filepath.Join(dir, e.Name())
		if kind == "up" {
			m.UpPath = path
		} else {
			m.DownPath = path
		}
	}

	versions := make([]string, 0, len(byVersion))
	for v := range byVersion {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool {
		ni, erri := strconv.ParseUint(versions[i], 10, 64)
		nj, errj := strconv.ParseUint(versions[j], 10, 64)
		if erri == nil && errj == nil {
			return ni < nj
		}
		return versions[i] < versions[j]
	})

	migrations := make([]Migration, 0, len(versions))
	for _, v := range versions {
		migrations = append(migrations, *byVersion[v])
	}
	return migrations, nil
}

// Plan returns the migrations Apply would run, without running them.
// For up, steps caps the count (0 = all); for down, steps is how many to roll
// back (0 = 1).
func Plan(ctx context.Context, db *sql.DB, migrations []Migration, d Direction, steps int) ([]Migration, error) {
	if err := ensureSchemaMigrationsTable(ctx, db); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations: %w", err)
	}
	applied, err := AppliedVersions(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("load applied versions: %w", err)
	}
	switch d {
	case Up:
		return pendingUp(migrations, applied, steps), nil
	case Down:
		return pendingDown(migrations, applied, steps)
	default:
		return nil, fmt.Errorf("unknown direction %q (want: up | down)", d)
	}
}

// Apply runs the planned migrations in order, one transaction each, calling
// onApplied (may be nil) after each success. It returns what was applied.
func Apply(ctx context.Context, db *sql.DB, migrations []Migration, d Direction, steps int, onApplied func(Migration)) ([]Migration, error) {
	plan, err := Plan(ctx, db, migrations, d, steps)
	if err != nil {
		return nil, err
	}
	for i, m := range plan {
		if err := applyOne(ctx, db, m, d); err != nil {
			return plan[:i], fmt.Errorf("%s: %w", filepath.Base(m.Path(d)), err)
		}
		if onApplied != nil {
			onApplied(m)
		}
	}
	return plan, nil
}

// UpAll applies every pending migration in dir. Convenience for tests.
func UpAll(ctx context.Context, db *sql.DB, dir string) error {
	ms, err := Load(dir)
	if err != nil {
		return err
	}
	_, err = Apply(ctx, db, ms, Up, 0, nil)
	return err
}

// AppliedVersions returns the set of versions recorded in schema_migrations.
func AppliedVersions(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func ensureSchemaMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT        PRIMARY KEY,
			name       TEXT        NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

func applyOne(ctx context.Context, db *sql.DB, m Migration, d Direction) error {
	path := m.Path(d)
	if path == "" {
		return fmt.Errorf("no %s.sql file for version %s", d, m.Version)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return err
	}

	if d == Up {
		_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.Version, m.Name)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, m.Version)
	}
	if err != nil {
		return err
	}

	return tx.Commit()
}

// pendingUp returns not-yet-applied migrations in ascending version order,
// capped at steps (0 = no cap).
func pendingUp(migrations []Migration, applied map[string]bool, steps int) []Migration {
	var plan []Migration
	for _, m := range migrations {
		if !applied[m.Version] {
			plan = append(plan, m)
		}
		if steps > 0 && len(plan) == steps {
			break
		}
	}
	return plan
}

// pendingDown returns applied migrations in descending version order, capped
// at steps (0 defaults to 1, the usual "roll back the last migration").
func pendingDown(migrations []Migration, applied map[string]bool, steps int) ([]Migration, error) {
	if steps <= 0 {
		steps = 1
	}

	byVersion := make(map[string]Migration, len(migrations))
	for _, m := range migrations {
		byVersion[m.Version] = m
	}

	appliedOrdered := make([]string, 0, len(applied))
	for v := range applied {
		appliedOrdered = append(appliedOrdered, v)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(appliedOrdered)))

	var plan []Migration
	for _, v := range appliedOrdered {
		m, ok := byVersion[v]
		if !ok {
			return nil, fmt.Errorf("version %s is applied but has no migration file in this directory", v)
		}
		plan = append(plan, m)
		if len(plan) == steps {
			break
		}
	}
	return plan, nil
}
