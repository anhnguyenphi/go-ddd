// Package postgres is the production implementation of the customer domain
// Repository, backed by database/sql. It enlists in the caller's transaction via
// shared/infrastructure/transaction, so aggregate writes and the outbox write
// commit together.
//
// It compiles with no driver present; register one (pgx stdlib / lib/pq) in the
// process main and set DATABASE_URL to activate it (see internal/bootstrap).
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/example/myapp/internal/customer/domain"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/internal/shared/infrastructure/transaction"
)

// Repository reads/writes the `customers` table.
type Repository struct {
	db *sql.DB
}

// New builds the Repository.
func New(db *sql.DB) *Repository { return &Repository{db: db} }

var _ domain.Repository = (*Repository)(nil)

// querier is satisfied by both *sql.DB and *sql.Tx.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// conn returns the ambient transaction if one is in flight, else the pool.
func (r *Repository) conn(ctx context.Context) querier {
	if tx, ok := transaction.FromContext(ctx); ok {
		return tx
	}
	return r.db
}

// FindByID loads one customer.
func (r *Repository) FindByID(ctx context.Context, id domain.CustomerID) (*domain.Customer, error) {
	const q = `
		SELECT id, name, email, status, created_at, version
		FROM customers WHERE id = $1`

	var s domain.Snapshot
	var createdAt time.Time
	err := r.conn(ctx).QueryRowContext(ctx, q, id.String()).
		Scan(&s.ID, &s.Name, &s.Email, &s.Status, &createdAt, &s.Version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, shareddomain.ErrNotFound
	case err != nil:
		return nil, err
	}
	s.CreatedAt = createdAt.UTC()
	return domain.FromSnapshot(s)
}

// Save upserts the aggregate with an optimistic-concurrency guard on version.
func (r *Repository) Save(ctx context.Context, customer *domain.Customer) error {
	s := customer.ToSnapshot()
	const q = `
		INSERT INTO customers (id, name, email, status, created_at, version)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			email = EXCLUDED.email,
			status = EXCLUDED.status,
			version = EXCLUDED.version
		WHERE customers.version < EXCLUDED.version`

	res, err := r.conn(ctx).ExecContext(ctx, q,
		s.ID, s.Name, s.Email, s.Status, s.CreatedAt, s.Version)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shareddomain.ErrConflict
	}
	return nil
}

// ExistsByEmail backs the email-uniqueness policy.
func (r *Repository) ExistsByEmail(ctx context.Context, email domain.Email) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM customers WHERE email = $1)`
	var exists bool
	if err := r.conn(ctx).QueryRowContext(ctx, q, email.String()).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
