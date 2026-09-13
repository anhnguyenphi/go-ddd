// Package memory is an in-memory implementation of the order domain
// Repository. It is the default wiring so the app runs with no database.
package memory

import (
	"context"
	"sync"

	"github.com/example/myapp/internal/order/domain"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// Repository stores orders in a map, guarded by a mutex.
type Repository struct {
	mu   sync.RWMutex
	byID map[string]*domain.Order
}

// New builds an empty Repository.
func New() *Repository {
	return &Repository{byID: make(map[string]*domain.Order)}
}

var _ domain.Repository = (*Repository)(nil)

// FindByID returns the order or shared/domain.ErrNotFound.
func (r *Repository) FindByID(_ context.Context, id domain.OrderID) (*domain.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	order, ok := r.byID[id.String()]
	if !ok {
		return nil, shareddomain.ErrNotFound
	}
	return order, nil
}

// Save inserts or updates the aggregate.
func (r *Repository) Save(_ context.Context, order *domain.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[order.ID().String()] = order
	return nil
}
