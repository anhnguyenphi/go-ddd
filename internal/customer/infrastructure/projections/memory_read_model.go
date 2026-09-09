// Package projections builds and serves the customer context's query-side read
// models by consuming its own integration events. This keeps the write model
// (aggregate) and read model (DTO) independently shaped — CQRS.
package projections

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	custapp "github.com/example/myapp/internal/customer/application"
	"github.com/example/myapp/internal/customer/application/dto"
	"github.com/example/myapp/internal/customer/infrastructure/messaging"
	"github.com/example/myapp/internal/eventbus"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/pkg/pagination"
)

// MemoryReadModel is an in-memory customer read model. In production this would
// be a denormalised table (or a search index) updated by the same handlers.
type MemoryReadModel struct {
	mu   sync.RWMutex
	rows map[string]dto.Customer
}

// NewMemoryReadModel builds an empty read model.
func NewMemoryReadModel() *MemoryReadModel {
	return &MemoryReadModel{rows: make(map[string]dto.Customer)}
}

var _ custapp.ReadModel = (*MemoryReadModel)(nil)

// Register subscribes the projection's handlers to the bus.
func (m *MemoryReadModel) Register(bus eventbus.Subscriber) error {
	if err := bus.Subscribe(messaging.EventCustomerCreated, m.onCreated); err != nil {
		return err
	}
	return bus.Subscribe(messaging.EventCustomerEmailChanged, m.onEmailChanged)
}

func (m *MemoryReadModel) onCreated(_ context.Context, e eventbus.Event) error {
	var p struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[p.ID] = dto.Customer{
		ID:        p.ID,
		Name:      p.Name,
		Email:     p.Email,
		Status:    "active",
		CreatedAt: nonZero(e.OccurredAt),
	}
	return nil
}

func (m *MemoryReadModel) onEmailChanged(_ context.Context, e eventbus.Event) error {
	var p struct {
		ID       string `json:"id"`
		NewEmail string `json:"new_email"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.rows[p.ID]; ok {
		row.Email = p.NewEmail
		m.rows[p.ID] = row
	}
	return nil
}

// GetByID implements custapp.ReadModel.
func (m *MemoryReadModel) GetByID(_ context.Context, id string) (dto.Customer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row, ok := m.rows[id]
	if !ok {
		return dto.Customer{}, shareddomain.ErrNotFound
	}
	return row, nil
}

// List implements custapp.ReadModel.
func (m *MemoryReadModel) List(_ context.Context, page pagination.Page) (pagination.Result[dto.Customer], error) {
	m.mu.RLock()
	all := make([]dto.Customer, 0, len(m.rows))
	for _, row := range m.rows {
		all = append(all, row)
	}
	m.mu.RUnlock()

	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID < all[j].ID
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})

	total := len(all)
	lo := min(page.Offset, total)
	hi := min(lo+page.Limit, total)
	return pagination.Result[dto.Customer]{
		Items:  all[lo:hi],
		Total:  total,
		Limit:  page.Limit,
		Offset: page.Offset,
	}, nil
}

func nonZero(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
