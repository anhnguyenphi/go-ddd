//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/example/myapp/internal/customer/domain"
	custpostgres "github.com/example/myapp/internal/customer/infrastructure/persistence/postgres"
	"github.com/example/myapp/internal/customer/infrastructure/persistence/repotest"
	"github.com/example/myapp/internal/customer/infrastructure/projections"
	"github.com/example/myapp/internal/customer/infrastructure/projections/projectiontest"
	"github.com/example/myapp/internal/shared/infrastructure/transaction"
	"github.com/example/myapp/internal/testsupport/containers"
)

func TestPostgresRepository_Conformance(t *testing.T) {
	repotest.Run(t, func(t *testing.T) domain.Repository {
		return custpostgres.New(containers.PostgresDB(t))
	})
}

func TestPostgresReadModel_Conformance(t *testing.T) {
	projectiontest.Run(t, func(t *testing.T) projections.ReadModel {
		return projections.NewPostgresReadModel(containers.PostgresDB(t))
	})
}

func newCustomer(t *testing.T, id, email string) *domain.Customer {
	t.Helper()
	e, err := domain.NewEmail(email)
	if err != nil {
		t.Fatal(err)
	}
	c, err := domain.NewCustomer(domain.MustCustomerID(id), "Tx", e, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The SQL unit of work must make repository writes inside it all-or-nothing.
func TestSQLUnitOfWork_RollsBackOnError(t *testing.T) {
	db := containers.PostgresDB(t)
	repo := custpostgres.New(db)
	uow := transaction.SQL{DB: db}
	ctx := context.Background()
	boom := errors.New("boom")

	err := uow.Within(ctx, func(ctx context.Context) error {
		if err := repo.Save(ctx, newCustomer(t, "tx-rollback", "rollback@example.test")); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Within err = %v, want boom", err)
	}
	if _, err := repo.FindByID(ctx, domain.MustCustomerID("tx-rollback")); err == nil {
		t.Fatal("row survived a rolled-back unit of work")
	}

	if err := uow.Within(ctx, func(ctx context.Context) error {
		return repo.Save(ctx, newCustomer(t, "tx-commit", "commit@example.test"))
	}); err != nil {
		t.Fatalf("Within commit: %v", err)
	}
	if _, err := repo.FindByID(ctx, domain.MustCustomerID("tx-commit")); err != nil {
		t.Fatalf("committed row missing: %v", err)
	}
}

// The unique index on customers.email is the storage-level backstop for the
// email-uniqueness policy (which races under concurrency).
func TestPostgres_EmailUniqueAtStorageLevel(t *testing.T) {
	db := containers.PostgresDB(t)
	repo := custpostgres.New(db)
	ctx := context.Background()
	if err := repo.Save(ctx, newCustomer(t, "u-1", "same@example.test")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, newCustomer(t, "u-2", "same@example.test")); err == nil {
		t.Fatal("second customer with the same email was stored")
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM customers`).Scan(&n); err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}
