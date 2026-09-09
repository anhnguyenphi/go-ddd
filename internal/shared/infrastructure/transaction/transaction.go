// Package transaction provides UnitOfWork implementations and a helper for
// propagating a *sql.Tx through context so repositories can enlist in the
// caller's transaction without taking it as a parameter.
package transaction

import (
	"context"
	"database/sql"
)

type ctxKey struct{}

// WithTx returns a child context carrying tx.
func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

// FromContext returns the *sql.Tx enlisted on ctx, if any.
func FromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(ctxKey{}).(*sql.Tx)
	return tx, ok
}

// Noop is an in-memory UnitOfWork: it runs fn directly and treats a nil error
// as "commit". Used by the default all-in-memory wiring.
type Noop struct{}

func (Noop) Within(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// SQL is a database-backed UnitOfWork. It opens a transaction, puts it on the
// context, and commits or rolls back based on fn's result. Repositories used by
// fn must call FromContext to enlist.
type SQL struct{ DB *sql.DB }

func (s SQL) Within(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err = fn(WithTx(ctx, tx)); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return rbErr
		}
		return err
	}
	return tx.Commit()
}
