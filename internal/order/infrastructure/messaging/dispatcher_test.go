package messaging_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/order/domain"
	"github.com/example/myapp/internal/order/infrastructure/messaging"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

type capturePublisher struct{ got []eventbus.Event }

func (c *capturePublisher) Publish(_ context.Context, evs ...eventbus.Event) error {
	c.got = append(c.got, evs...)
	return nil
}

type unmappedEvent struct{}

func (unmappedEvent) EventName() string     { return "order.unmapped" }
func (unmappedEvent) OccurredAt() time.Time { return time.Time{} }
func (unmappedEvent) AggregateID() string   { return "o-1" }

func TestDispatcher_ForwardsOrderPlacedAsIntegrationEvent(t *testing.T) {
	pub := &capturePublisher{}
	d := messaging.NewDispatcher(pub)
	placed := domain.OrderPlaced{
		OrderID: "o-1", CustomerID: "c-1", ProductName: "Tea",
		Amount: 450, Currency: "USD", At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if err := d.Publish(context.Background(), []shareddomain.DomainEvent{placed, unmappedEvent{}}...); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(pub.got) != 1 {
		t.Fatalf("forwarded %d events, want 1 (unmapped events are dropped)", len(pub.got))
	}
	e := pub.got[0]
	if e.Name != "order.v1.placed" || e.Source != "order" || e.AggregateID != "o-1" {
		t.Fatalf("envelope = %+v", e)
	}
	var payload map[string]any
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["product_name"] != "Tea" || payload["amount"] != float64(450) || payload["currency"] != "USD" {
		t.Fatalf("payload = %s", e.Payload)
	}
}
