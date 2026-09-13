package commands

import (
	"context"

	"github.com/example/myapp/internal/order/domain"
)

// PlaceOrder places a new order for a customer.
type PlaceOrder struct {
	CustomerID string
}

// PlaceOrderResult carries the identity of the new order.
type PlaceOrderResult struct {
	OrderID string
}

// PlaceOrderHandler executes PlaceOrder.
type PlaceOrderHandler struct{ deps Deps }

// NewPlaceOrderHandler wires the handler.
func NewPlaceOrderHandler(d Deps) *PlaceOrderHandler {
	return &PlaceOrderHandler{deps: d}
}

// Handle verifies the customer synchronously — via a direct, in-process call
// into the customer context (see application.CustomerVerifier) — before the
// order aggregate is even built. An order cannot be placed at all for an
// unknown customer, so unlike the customer.created -> notification flow,
// there is no useful "eventually" here: the caller needs a yes/no now.
func (h *PlaceOrderHandler) Handle(ctx context.Context, cmd PlaceOrder) (PlaceOrderResult, error) {
	if err := h.deps.Customer.Verify(ctx, cmd.CustomerID); err != nil {
		return PlaceOrderResult{}, err
	}

	id, err := domain.NewOrderID(h.deps.IDs.NewID())
	if err != nil {
		return PlaceOrderResult{}, err
	}
	order, err := domain.Place(id, cmd.CustomerID, h.deps.Clock.Now())
	if err != nil {
		return PlaceOrderResult{}, err
	}
	if err := h.deps.Repo.Save(ctx, order); err != nil {
		return PlaceOrderResult{}, err
	}
	return PlaceOrderResult{OrderID: id.String()}, nil
}
