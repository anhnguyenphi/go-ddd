// Package domain is the customer bounded context's domain layer: aggregates,
// value objects, domain events, repository interfaces and domain services.
//
// It depends only on the standard library and shared/domain. It must never
// import a database, HTTP, the event bus, or another bounded context.
package domain

import (
	"time"

	"github.com/example/myapp/internal/customer/domain/events"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// Customer is the aggregate root of this context. All state transitions go
// through its behaviour methods, which enforce invariants and record domain
// events. Timestamps are passed in by the application layer (which owns the
// clock) to keep the domain deterministic.
type Customer struct {
	shareddomain.AggregateRoot

	id        CustomerID
	name      string
	email     Email
	status    Status
	createdAt time.Time
	version   int // optimistic-concurrency token, bumped per mutation
}

// NewCustomer registers a brand-new customer and records CustomerCreated.
func NewCustomer(id CustomerID, name string, email Email, occurredAt time.Time) (*Customer, error) {
	if id.IsZero() {
		return nil, shareddomain.Invalid("customer id is required")
	}
	if err := validateName(name); err != nil {
		return nil, err
	}

	c := &Customer{
		id:        id,
		name:      name,
		email:     email,
		status:    StatusActive,
		createdAt: occurredAt,
		version:   1,
	}
	c.RecordEvent(events.CustomerCreated{
		CustomerID: id.String(),
		Name:       name,
		Email:      email.String(),
		At:         occurredAt,
	})
	return c, nil
}

// ChangeEmail updates the address. It is a no-op (no event) if the address is
// unchanged. Deactivated customers cannot change their email.
func (c *Customer) ChangeEmail(newEmail Email, occurredAt time.Time) error {
	if c.status == StatusDeactivated {
		return shareddomain.ErrConflict
	}
	if c.email.Equals(newEmail) {
		return nil
	}
	old := c.email
	c.email = newEmail
	c.version++
	c.RecordEvent(events.CustomerEmailChanged{
		CustomerID: c.id.String(),
		OldEmail:   old.String(),
		NewEmail:   newEmail.String(),
		At:         occurredAt,
	})
	return nil
}

// Deactivate disables the customer. Idempotent.
func (c *Customer) Deactivate(reason string, occurredAt time.Time) error {
	if c.status == StatusDeactivated {
		return nil
	}
	c.status = StatusDeactivated
	c.version++
	c.RecordEvent(events.CustomerDeactivated{
		CustomerID: c.id.String(),
		Reason:     reason,
		At:         occurredAt,
	})
	return nil
}

// Getters — the aggregate exposes state read-only.
func (c *Customer) ID() CustomerID       { return c.id }
func (c *Customer) Name() string         { return c.name }
func (c *Customer) Email() Email         { return c.email }
func (c *Customer) Status() Status       { return c.status }
func (c *Customer) CreatedAt() time.Time { return c.createdAt }
func (c *Customer) Version() int         { return c.version }

func validateName(name string) error {
	switch {
	case name == "":
		return shareddomain.Invalid("customer name is required")
	case len(name) > 200:
		return shareddomain.Invalid("customer name must be 200 characters or fewer")
	default:
		return nil
	}
}
