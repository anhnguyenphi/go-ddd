// Package messaging translates the order context's domain events into the
// versioned integration events that are its public contract, and hosts the
// adapter that pushes them onto the bus (via the outbox).
package messaging

import (
	"encoding/json"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/order/domain"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/internal/shared/infrastructure/id"
)

// Source identifies this context on the bus.
const Source = "order"

// Integration event names — the versioned public contract. Consumers
// subscribe to these strings; changing a payload shape means a new vN.
const (
	EventOrderPlaced = "order.v1.placed"
)

// orderPlacedV1 is the wire payload for EventOrderPlaced. Amount is in minor
// units of Currency (1999 + "EUR" = 19.99 EUR).
type orderPlacedV1 struct {
	ID          string `json:"id"`
	CustomerID  string `json:"customer_id"`
	ProductName string `json:"product_name"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
}

// ToIntegrationEvent maps one domain event. The bool is false for domain
// events that are intentionally not published outside the context.
func ToIntegrationEvent(evt shareddomain.DomainEvent) (eventbus.Event, bool) {
	switch e := evt.(type) {
	case domain.OrderPlaced:
		body, err := json.Marshal(orderPlacedV1{
			ID: e.OrderID, CustomerID: e.CustomerID, ProductName: e.ProductName,
			Amount: e.Amount, Currency: e.Currency,
		})
		if err != nil {
			return eventbus.Event{}, false
		}
		return eventbus.Event{
			ID:          id.New(),
			Name:        EventOrderPlaced,
			Source:      Source,
			AggregateID: e.OrderID,
			OccurredAt:  e.At.UTC(),
			Version:     1,
			Payload:     body,
			Metadata:    map[string]string{},
		}, true
	default:
		return eventbus.Event{}, false
	}
}
