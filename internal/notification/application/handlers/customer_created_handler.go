// Package handlers reacts to integration events from other bounded contexts.
// It depends only on published event contracts (names + payload shapes), never
// on another context's Go packages.
package handlers

import (
	"context"
	"encoding/json"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/notification/domain"
)

// EventCustomerCreated is the customer context's public contract this handler
// consumes. It is intentionally a plain string constant, duplicated from the
// producer — the coupling is the versioned contract, not the code.
const EventCustomerCreated = "customer.v1.created"

// customerCreatedV1 mirrors the producer's published payload shape.
type customerCreatedV1 struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// CustomerCreated turns a customer.v1.created event into a welcome notification.
type CustomerCreated struct {
	notifications domain.NotificationService
}

// NewCustomerCreated wires the handler.
func NewCustomerCreated(n domain.NotificationService) *CustomerCreated {
	return &CustomerCreated{notifications: n}
}

// Handle is an eventbus.Handler. It must be idempotent: at-least-once delivery
// means it can be called more than once for the same Event.ID.
func (h *CustomerCreated) Handle(ctx context.Context, e eventbus.Event) error {
	var p customerCreatedV1
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	return h.notifications.SendWelcome(ctx, p.Email, p.Name)
}
