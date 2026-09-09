package messaging

import (
	"context"
	"log/slog"

	"github.com/example/myapp/internal/eventbus"
)

// InboundHandlers is where this context reacts to *other* contexts' integration
// events. It depends only on published event contracts (event name + payload
// shape), never on another context's Go packages.
type InboundHandlers struct {
	logger *slog.Logger
}

// NewInboundHandlers builds the handler set.
func NewInboundHandlers(logger *slog.Logger) *InboundHandlers {
	return &InboundHandlers{logger: logger}
}

// Register subscribes the handlers to the bus. This wires one illustrative
// cross-context reaction; add more the same way.
func (h *InboundHandlers) Register(bus eventbus.Subscriber) error {
	return bus.Subscribe("order.v1.placed", h.onOrderPlaced)
}

// onOrderPlaced is a placeholder reaction to the order context's public event.
// A real implementation would issue a command (e.g. mark the customer as
// "converted") rather than just logging.
func (h *InboundHandlers) onOrderPlaced(ctx context.Context, e eventbus.Event) error {
	h.logger.InfoContext(ctx, "customer context observed order.v1.placed",
		"event_id", e.ID, "aggregate_id", e.AggregateID)
	return nil
}
