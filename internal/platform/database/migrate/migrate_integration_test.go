//go:build integration

package migrate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/example/myapp/internal/platform/database/migrate"
	"github.com/example/myapp/internal/testsupport/containers"
)

// TestMain terminates the containers this package's tests started.
func TestMain(m *testing.M) { os.Exit(containers.RunMain(m)) }

// Every migration must roll back cleanly and re-apply: up (done by the
// template) -> down all -> up all. Automates step 6 of the definition of done.
func TestMigrations_DownAllThenUpAgain(t *testing.T) {
	ctx := context.Background()
	db, err := migrate.Open(ctx, containers.PostgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ms, err := migrate.Load(filepath.Join(containers.RepoRoot(), "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 {
		t.Fatal("no migrations found")
	}
	for _, m := range ms {
		if m.DownPath == "" {
			t.Errorf("migration %s_%s has no down file", m.Version, m.Name)
		}
	}

	tables := func() int {
		t.Helper()
		var n int
		err := db.QueryRowContext(ctx, `
			SELECT count(*) FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name <> 'schema_migrations'`).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := tables()
	if before == 0 {
		t.Fatal("template database has no tables; migrations did not run")
	}

	down, err := migrate.Apply(ctx, db, ms, migrate.Down, len(ms), nil)
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	if len(down) != len(ms) {
		t.Fatalf("rolled back %d of %d", len(down), len(ms))
	}
	if n := tables(); n != 0 {
		t.Fatalf("%d tables left after rolling everything back", n)
	}

	up, err := migrate.Apply(ctx, db, ms, migrate.Up, 0, nil)
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if len(up) != len(ms) || tables() != before {
		t.Fatalf("re-applied %d migrations, %d tables; want %d, %d", len(up), tables(), len(ms), before)
	}

	// Up again is a no-op.
	if again, err := migrate.Apply(ctx, db, ms, migrate.Up, 0, nil); err != nil || len(again) != 0 {
		t.Fatalf("second up applied %d (err %v), want 0", len(again), err)
	}
}
