package application

import "context"

// UnitOfWork runs fn as a single atomic unit. Everything fn does through the
// context it receives — aggregate saves *and* the outbox write — commits or
// rolls back together.
//
// Implementations:
//   - shared/infrastructure/transaction.Noop — in-memory, always "commits"
//   - shared/infrastructure/transaction.SQL  — wraps *sql.DB, puts *sql.Tx in ctx
type UnitOfWork interface {
	Within(ctx context.Context, fn func(ctx context.Context) error) error
}
