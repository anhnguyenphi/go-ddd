package commands

import (
	"context"

	"github.com/example/myapp/internal/order/domain"
)

// PlaceOrder places a new order for a customer: one product, for one amount.
type PlaceOrder struct {
	CustomerID  string
	ProductName string
	Amount      int64 // minor units of Currency
	Currency    string
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

// Handle validates the product name and amount, verifies the customer synchronously — via a
// direct, in-process call into the customer context (see
// application.CustomerVerifier) — and only then builds the order. An order
// cannot be placed at all for an unknown customer, so unlike the
// customer.created -> notification flow, there is no useful "eventually"
// here: the caller needs a yes/no now. The order and its OrderPlaced event
// are saved and published in one unit of work.
func (h *PlaceOrderHandler) Handle(ctx context.Context, cmd PlaceOrder) (PlaceOrderResult, error) {
	// Input is checked first, so invalid input never costs a cross-context
	// call and is a 400 even for an unknown customer.
	productName, err := domain.NewProductName(cmd.ProductName)
	if err != nil {
		return PlaceOrderResult{}, err
	}
	amount, err := domain.NewMoney(cmd.Amount, cmd.Currency)
	if err != nil {
		return PlaceOrderResult{}, err
	}
	if err := h.deps.Customer.Verify(ctx, cmd.CustomerID); err != nil {
		return PlaceOrderResult{}, err
	}

	id, err := domain.NewOrderID(h.deps.IDs.NewID())
	if err != nil {
		return PlaceOrderResult{}, err
	}
	order, err := domain.Place(id, cmd.CustomerID, productName, amount, h.deps.Clock.Now())
	if err != nil {
		return PlaceOrderResult{}, err
	}

	err = h.deps.UoW.Within(ctx, func(ctx context.Context) error {
		if err := h.deps.Repo.Save(ctx, order); err != nil {
			return err
		}
		return h.deps.Events.Publish(ctx, order.PullEvents()...)
	})
	if err != nil {
		return PlaceOrderResult{}, err
	}
	return PlaceOrderResult{OrderID: id.String()}, nil
}
