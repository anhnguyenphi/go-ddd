// Package messaging translates the customer context's internal domain events
// into the versioned integration events that are its public contract, and hosts
// the adapter that pushes them onto the bus (via the outbox).
package messaging

import (
	"encoding/json"
	"time"

	"github.com/example/myapp/internal/customer/domain/events"
	"github.com/example/myapp/internal/eventbus"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/internal/shared/infrastructure/id"
)

// Source identifies this context on the bus.
const Source = "customer"

// Integration event names — the versioned public contract. Consumers subscribe
// to these strings; changing a payload shape means a new vN.
const (
	EventCustomerCreated      = "customer.v1.created"
	EventCustomerEmailChanged = "customer.v1.email_changed"
	EventCustomerDeactivated  = "customer.v1.deactivated"
)

// customerCreatedV1 is the wire payload for EventCustomerCreated.
type customerCreatedV1 struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type customerEmailChangedV1 struct {
	ID       string `json:"id"`
	OldEmail string `json:"old_email"`
	NewEmail string `json:"new_email"`
}

type customerDeactivatedV1 struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// ToIntegrationEvent maps one domain event. The bool is false for domain events
// that are intentionally not published outside the context.
func ToIntegrationEvent(evt shareddomain.DomainEvent) (eventbus.Event, bool) {
	base := func(name string, aggregateID string, occurredAt time.Time, payload any) (eventbus.Event, bool) {
		body, err := json.Marshal(payload)
		if err != nil {
			return eventbus.Event{}, false
		}
		return eventbus.Event{
			ID:          id.New(),
			Name:        name,
			Source:      Source,
			AggregateID: aggregateID,
			OccurredAt:  occurredAt.UTC(),
			Version:     1,
			Payload:     body,
			Metadata:    map[string]string{},
		}, true
	}

	switch e := evt.(type) {
	case events.CustomerCreated:
		return base(EventCustomerCreated, e.CustomerID, e.At,
			customerCreatedV1{ID: e.CustomerID, Name: e.Name, Email: e.Email})
	case events.CustomerEmailChanged:
		return base(EventCustomerEmailChanged, e.CustomerID, e.At,
			customerEmailChangedV1{ID: e.CustomerID, OldEmail: e.OldEmail, NewEmail: e.NewEmail})
	case events.CustomerDeactivated:
		return base(EventCustomerDeactivated, e.CustomerID, e.At,
			customerDeactivatedV1{ID: e.CustomerID, Reason: e.Reason})
	default:
		return eventbus.Event{}, false
	}
}
