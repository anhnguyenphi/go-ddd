package domain

import "context"

// Repository is the order aggregate's persistence port. It is defined here,
// in the domain, and implemented in infrastructure/persistence/*.
type Repository interface {
	// FindByID returns the order or shared/domain.ErrNotFound.
	FindByID(ctx context.Context, id OrderID) (*Order, error)
	// Save inserts or updates the aggregate.
	Save(ctx context.Context, order *Order) error
}
