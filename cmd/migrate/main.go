// Command migrate is the schema-migration entrypoint. It discovers and orders
// *.up.sql / *.down.sql files, then — when DATABASE_URL is set — applies the
// pending ones inside transactions, tracking progress in a schema_migrations
// table. With DATABASE_URL unset it only prints the plan. The logic lives in
// internal/platform/database/migrate so tests apply migrations the same way.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/example/myapp/internal/platform/database/migrate"
)

func main() {
	dir := flag.String("dir", "migrations", "directory containing *.sql migrations")
	steps := flag.Int("steps", 0, "up: max pending migrations to apply (0 = all); down: migrations to roll back (0 = 1)")
	flag.Parse()

	direction := migrate.Direction(flag.Arg(0))
	if direction == "" {
		direction = migrate.Up
	}
	if direction != migrate.Up && direction != migrate.Down {
		fail(fmt.Errorf("unknown direction %q (want: up | down)", direction))
	}

	migrations, err := migrate.Load(*dir)
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

func run(dsn string, direction migrate.Direction, steps int, migrations []migrate.Migration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := migrate.Open(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer db.Close()

	plan, err := migrate.Plan(ctx, db, migrations, direction, steps)
	if err != nil {
		return err
	}
	if len(plan) == 0 {
		fmt.Println("nothing to do")
		return nil
	}

	fmt.Printf("applying %d migration(s) (%s):\n", len(plan), direction)
	_, err = migrate.Apply(ctx, db, migrations, direction, steps, func(m migrate.Migration) {
		fmt.Printf("  ok  %s\n", filepath.Base(m.Path(direction)))
	})
	return err
}

func printPlan(direction migrate.Direction, dir string, migrations []migrate.Migration) {
	fmt.Printf("migration plan (%s) from %s:\n", direction, dir)
	if len(migrations) == 0 {
		fmt.Println("  (no migrations found)")
		return
	}

	ordered := make([]migrate.Migration, len(migrations))
	copy(ordered, migrations)
	if direction == migrate.Down {
		for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
			ordered[i], ordered[j] = ordered[j], ordered[i]
		}
	}
	for i, m := range ordered {
		path := m.Path(direction)
		if path == "" {
			path = fmt.Sprintf("%s_%s.%s.sql (missing)", m.Version, m.Name, direction)
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
