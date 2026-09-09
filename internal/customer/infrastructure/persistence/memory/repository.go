// Package memory is an in-memory implementation of the customer domain
// Repository. It is the default wiring so the app runs with no database. It is
// also useful as a fast fake in application-layer tests.
package memory

import (
	"context"
	"sync"

	"github.com/example/myapp/internal/customer/domain"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// Repository stores customers as snapshots in a map, guarded by a mutex.
type Repository struct {
	mu      sync.RWMutex
	byID    map[string]domain.Snapshot
	byEmail map[string]string // email -> id
}

// New builds an empty Repository.
func New() *Repository {
	return &Repository{
		byID:    make(map[string]domain.Snapshot),
		byEmail: make(map[string]string),
	}
}

var _ domain.Repository = (*Repository)(nil)

// FindByID reconstructs the aggregate from its snapshot.
func (r *Repository) FindByID(_ context.Context, id domain.CustomerID) (*domain.Customer, error) {
	r.mu.RLock()
	snap, ok := r.byID[id.String()]
	r.mu.RUnlock()
	if !ok {
		return nil, shareddomain.ErrNotFound
	}
	return domain.FromSnapshot(snap)
}

// Save writes the aggregate's snapshot, enforcing optimistic concurrency.
func (r *Repository) Save(_ context.Context, customer *domain.Customer) error {
	snap := customer.ToSnapshot()

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.byID[snap.ID]; ok && existing.Version > snap.Version {
		return shareddomain.ErrConflict
	}
	// keep the email index consistent
	if existing, ok := r.byID[snap.ID]; ok && existing.Email != snap.Email {
		delete(r.byEmail, existing.Email)
	}
	r.byID[snap.ID] = snap
	r.byEmail[snap.Email] = snap.ID
	return nil
}

// ExistsByEmail backs the email-uniqueness policy.
func (r *Repository) ExistsByEmail(_ context.Context, email domain.Email) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.byEmail[email.String()]
	return ok, nil
}
