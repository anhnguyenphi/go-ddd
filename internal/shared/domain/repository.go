package domain

import "context"

// Repository is the minimal contract every aggregate repository satisfies.
// Concrete repositories live in each context's domain package (as interfaces)
// and are implemented in that context's infrastructure package.
//
// Repositories deal in whole aggregates only — never in rows, DTOs, or partial
// graphs. Query-side read models are a separate concern (see application ports).
type Repository[T any, ID comparable] interface {
	FindByID(ctx context.Context, id ID) (T, error)
	Save(ctx context.Context, aggregate T) error
}
