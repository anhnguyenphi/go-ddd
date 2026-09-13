// Command migrate is the schema-migration entrypoint. It discovers and orders
// *.up.sql / *.down.sql files, then — when DATABASE_URL is set — applies the
// pending ones inside transactions, tracking progress in a schema_migrations
// table. With DATABASE_URL unset it only prints the plan.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
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

var filenameRe = regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)

// migration groups the up/down pair discovered for one version.
type migration struct {
	version  string
	name     string
	upPath   string
	downPath string
}

func main() {
	dir := flag.String("dir", "migrations", "directory containing *.sql migrations")
	steps := flag.Int("steps", 0, "up: max pending migrations to apply (0 = all); down: migrations to roll back (0 = 1)")
	flag.Parse()

	direction := flag.Arg(0)
	if direction == "" {
		direction = "up"
	}
	if direction != "up" && direction != "down" {
		fail(fmt.Errorf("unknown direction %q (want: up | down)", direction))
	}

	migrations, err := loadMigrations(*dir)
	if err != nil {
		fail(err)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		printPlan(direction, *dir, migrations)
		fmt.Println("\nDATABASE_URL not set — plan only, nothing applied.")
		return
	}

	if err := run(dsn, direction, *steps, migrations); err != nil {
		fail(err)
	}
}

func run(dsn, direction string, steps int, migrations []migration) error {
	db, err := open(dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := ensureSchemaMigrationsTable(ctx, db); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return fmt.Errorf("load applied versions: %w", err)
	}

	var plan []migration
	switch direction {
	case "up":
		plan = pendingUp(migrations, applied, steps)
	case "down":
		plan, err = pendingDown(migrations, applied, steps)
		if err != nil {
			return err
		}
	}

	if len(plan) == 0 {
		fmt.Println("nothing to do")
		return nil
	}

	fmt.Printf("applying %d migration(s) (%s):\n", len(plan), direction)
	for _, m := range plan {
		if err := applyOne(ctx, db, m, direction); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(pathFor(m, direction)), err)
		}
		fmt.Printf("  ok  %s\n", filepath.Base(pathFor(m, direction)))
	}
	return nil
}

// open configures the pgx stdlib driver for the simple query protocol, which
// (unlike the default extended protocol) allows a single Exec to run a
// migration file containing multiple ";"-separated statements.
func open(dsn string) (*sql.DB, error) {
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	connConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	db := stdlib.OpenDB(*connConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
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

func appliedVersions(ctx context.Context, db *sql.DB) (map[string]bool, error) {
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

func applyOne(ctx context.Context, db *sql.DB, m migration, direction string) error {
	path := pathFor(m, direction)
	if path == "" {
		return fmt.Errorf("no %s.sql file for version %s", direction, m.version)
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

	if direction == "up" {
		_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, m.version)
	}
	if err != nil {
		return err
	}

	return tx.Commit()
}

func pathFor(m migration, direction string) string {
	if direction == "up" {
		return m.upPath
	}
	return m.downPath
}

// pendingUp returns not-yet-applied migrations in ascending version order,
// capped at `steps` (0 = no cap).
func pendingUp(migrations []migration, applied map[string]bool, steps int) []migration {
	var plan []migration
	for _, m := range migrations {
		if !applied[m.version] {
			plan = append(plan, m)
		}
		if steps > 0 && len(plan) == steps {
			break
		}
	}
	return plan
}

// pendingDown returns applied migrations in descending version order, capped
// at `steps` (0 defaults to 1, the usual "roll back the last migration").
func pendingDown(migrations []migration, applied map[string]bool, steps int) ([]migration, error) {
	if steps <= 0 {
		steps = 1
	}

	byVersion := make(map[string]migration, len(migrations))
	for _, m := range migrations {
		byVersion[m.version] = m
	}

	var appliedOrdered []string
	for v := range applied {
		appliedOrdered = append(appliedOrdered, v)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(appliedOrdered)))

	var plan []migration
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

// loadMigrations discovers and pairs up *.up.sql / *.down.sql files, sorted
// by version ascending.
func loadMigrations(dir string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	byVersion := map[string]*migration{}
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
			m = &migration{version: version, name: name}
			byVersion[version] = m
		}
		path := filepath.Join(dir, e.Name())
		if kind == "up" {
			m.upPath = path
		} else {
			m.downPath = path
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

	migrations := make([]migration, 0, len(versions))
	for _, v := range versions {
		migrations = append(migrations, *byVersion[v])
	}
	return migrations, nil
}

func printPlan(direction, dir string, migrations []migration) {
	fmt.Printf("migration plan (%s) from %s:\n", direction, dir)
	if len(migrations) == 0 {
		fmt.Println("  (no migrations found)")
		return
	}

	ordered := make([]migration, len(migrations))
	copy(ordered, migrations)
	if direction == "down" {
		for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
			ordered[i], ordered[j] = ordered[j], ordered[i]
		}
	}
	for i, m := range ordered {
		path := pathFor(m, direction)
		if path == "" {
			path = fmt.Sprintf("%s_%s.%s.sql (missing)", m.version, m.name, direction)
		}
		fmt.Printf("  %2d. %s\n", i+1, path)
	}
}

func fail(err error) {
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "migrate: directory does not exist")
	} else {
		fmt.Fprintln(os.Stderr, "migrate:", err)
	}
	os.Exit(1)
}
