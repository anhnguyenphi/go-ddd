package order

import (
	"context"
	"testing"
	"time"

	orderv1 "github.com/example/myapp/api/proto/order/v1"
	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/shared/infrastructure/clock"
	"github.com/example/myapp/internal/shared/infrastructure/id"
	"github.com/example/myapp/internal/shared/infrastructure/transaction"
)

type allowAll struct{}

func (allowAll) Verify(context.Context, string) error { return nil }

type capturePublisher struct{ got []eventbus.Event }

func (c *capturePublisher) Publish(_ context.Context, evs ...eventbus.Event) error {
	c.got = append(c.got, evs...)
	return nil
}

// The module wires PlaceOrder's events through messaging.Dispatcher to the
// injected outbox publisher, so a placed order reaches the outbox as
// order.v1.placed.
func TestModule_PlaceOrderWritesOrderPlacedToOutbox(t *testing.T) {
	outbox := &capturePublisher{}
	m := New(Deps{
		UnitOfWork:      transaction.Noop{},
		OutboxPublisher: outbox,
		Clock:           clock.Fixed{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		IDs:             id.Fixed{Value: "o-1"},
		Customer:        allowAll{},
	})

	res, err := m.grpc.PlaceOrder(context.Background(), &orderv1.PlaceOrderRequest{
		CustomerId: "c-1", ProductName: "Tea", Amount: 450, Currency: "USD",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if len(outbox.got) != 1 {
		t.Fatalf("outbox got %d events, want 1", len(outbox.got))
	}
	if e := outbox.got[0]; e.Name != "order.v1.placed" || e.AggregateID != res.GetId() {
		t.Fatalf("outbox event = %+v", e)
	}
}
