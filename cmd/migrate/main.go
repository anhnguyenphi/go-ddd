// Command migrate is the schema-migration entrypoint.
//
// This skeleton ships a dependency-free planner: it discovers and orders
// migration files and prints the plan for `up` or `down`. Wire an actual
// migrator (golang-migrate, goose, atlas, ...) at the marked TODO once a
// database driver is added to the build.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	dir := flag.String("dir", "migrations", "directory containing *.sql migrations")
	flag.Parse()

	direction := flag.Arg(0)
	if direction == "" {
		direction = "up"
	}
	if direction != "up" && direction != "down" {
		fail(fmt.Errorf("unknown direction %q (want: up | down)", direction))
	}

	plan, err := buildPlan(*dir, direction)
	if err != nil {
		fail(err)
	}

	fmt.Printf("migration plan (%s) from %s:\n", direction, *dir)
	if len(plan) == 0 {
		fmt.Println("  (no migrations found)")
		return
	}
	for i, f := range plan {
		fmt.Printf("  %2d. %s\n", i+1, f)
	}

	if os.Getenv("DATABASE_URL") == "" {
		fmt.Println("\nDATABASE_URL not set — plan only, nothing applied.")
		return
	}
	// TODO: open *sql.DB with the registered driver and apply `plan` inside a
	// transaction, recording applied versions in a schema_migrations table.
	fmt.Println("\nDATABASE_URL is set, but no migrator is wired in this skeleton.")
	fmt.Println("See cmd/migrate/main.go for where to plug one in.")
}

// buildPlan returns the ordered list of migration files for the direction.
func buildPlan(dir, direction string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	suffix := "." + direction + ".sql"
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	if len(files) == 0 {
		return nil, nil
	}

	sort.Strings(files)
	if direction == "down" {
		// newest first when rolling back
		for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
			files[i], files[j] = files[j], files[i]
		}
	}
	return files, nil
}

func fail(err error) {
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "migrate: directory does not exist")
	} else {
		fmt.Fprintln(os.Stderr, "migrate:", err)
	}
	os.Exit(1)
}
