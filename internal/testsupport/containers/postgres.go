package containers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // "pgx" driver
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/example/myapp/internal/platform/database/migrate"
)

// templateDB holds the migrated schema; every test gets a clone of it.
const templateDB = "app_template"

var pg struct {
	once  sync.Once
	err   error
	base  *url.URL // DSN of the template DB
	admin *sql.DB  // connected to the "postgres" maintenance DB
	clone sync.Mutex
}

func startPostgres() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ctr, err := postgres.Run(ctx, PostgresImage,
		postgres.WithDatabase(templateDB),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if ctr != nil {
		onTerminate(ctr)
	}
	if err != nil {
		pg.err = fmt.Errorf("start postgres: %w", err)
		return
	}
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pg.err = err
		return
	}

	// Migrate the template once with the same code cmd/migrate uses.
	db, err := migrate.Open(ctx, dsn)
	if err != nil {
		pg.err = fmt.Errorf("connect template: %w", err)
		return
	}
	err = migrate.UpAll(ctx, db, filepath.Join(RepoRoot(), "migrations"))
	_ = db.Close()
	if err != nil {
		pg.err = fmt.Errorf("migrate template: %w", err)
		return
	}

	pg.base, _ = url.Parse(dsn)
	pg.admin, err = sql.Open("pgx", withDB(pg.base, "postgres"))
	if err != nil {
		pg.err = err
		return
	}
	_, pg.err = pg.admin.ExecContext(ctx, `ALTER DATABASE `+templateDB+` WITH ALLOW_CONNECTIONS false`)
}

func withDB(base *url.URL, name string) string {
	u := *base
	u.Path = "/" + name
	return u.String()
}

// PostgresDSN returns the DSN of a fresh, fully migrated database that only
// this test uses. It is dropped on cleanup. Cloning from a template takes
// milliseconds, so use one per test rather than sharing and truncating.
func PostgresDSN(t testing.TB) string {
	t.Helper()
	Require(t)
	pg.once.Do(startPostgres)
	if pg.err != nil {
		unavailable(t, "postgres", pg.err)
	}

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "t_" + hex.EncodeToString(b)

	ctx := context.Background()
	// CREATE DATABASE ... TEMPLATE fails if another clone is reading the
	// template at the same moment, so clones are serialized.
	pg.clone.Lock()
	_, err := pg.admin.ExecContext(ctx, `CREATE DATABASE `+name+` TEMPLATE `+templateDB)
	pg.clone.Unlock()
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pg.admin.ExecContext(context.Background(), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
	})
	return withDB(pg.base, name)
}

// PostgresDB opens a pool on a fresh database (see PostgresDSN), closed on
// cleanup. For adapter tests that don't need the whole application.
func PostgresDB(t testing.TB) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", PostgresDSN(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
