package domain

import "context"

// Repository is the customer aggregate's persistence port. It is defined here,
// in the domain, and implemented in infrastructure/persistence/*. It deals in
// whole aggregates only.
type Repository interface {
	// FindByID returns the customer or shared/domain.ErrNotFound.
	FindByID(ctx context.Context, id CustomerID) (*Customer, error)
	// Save inserts or updates the aggregate. Implementations should use
	// Customer.Version() for optimistic concurrency and return
	// shared/domain.ErrConflict on a version mismatch.
	Save(ctx context.Context, customer *Customer) error
	// ExistsByEmail supports the email-uniqueness domain policy.
	ExistsByEmail(ctx context.Context, email Email) (bool, error)
}
