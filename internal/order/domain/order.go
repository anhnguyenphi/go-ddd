// Package domain is the order context's domain layer (skeleton).
package domain

import (
	"time"

	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// OrderID identifies an Order aggregate.
type OrderID struct{ value string }

// NewOrderID validates a raw id.
func NewOrderID(v string) (OrderID, error) {
	if v == "" {
		return OrderID{}, shareddomain.Invalid("order id must not be empty")
	}
	return OrderID{value: v}, nil
}

// String returns the raw id.
func (id OrderID) String() string { return id.value }

// Order is the aggregate root: one product, for one amount, placed by one
// customer. There are no order lines yet.
type Order struct {
	shareddomain.AggregateRoot
	id          OrderID
	customerID  string
	productName ProductName
	amount      Money
	placedAt    time.Time
}

// Place creates a new order and records OrderPlaced. The product name and
// amount must be built with NewProductName and NewMoney.
func Place(id OrderID, customerID string, productName ProductName, amount Money, now time.Time) (*Order, error) {
	if customerID == "" {
		return nil, shareddomain.Invalid("order requires a customer")
	}
	if productName == (ProductName{}) {
		return nil, shareddomain.Invalid("order requires a product name")
	}
	if amount.IsZero() {
		return nil, shareddomain.Invalid("order requires an amount")
	}
	o := &Order{id: id, customerID: customerID, productName: productName, amount: amount, placedAt: now}
	o.RecordEvent(OrderPlaced{
		OrderID:     id.String(),
		CustomerID:  customerID,
		ProductName: productName.String(),
		Amount:      amount.Amount(),
		Currency:    amount.Currency(),
		At:          now,
	})
	return o, nil
}

// ID returns the aggregate identity.
func (o *Order) ID() OrderID { return o.id }

// ProductName returns what was bought.
func (o *Order) ProductName() ProductName { return o.productName }

// Amount returns what the customer pays.
func (o *Order) Amount() Money { return o.amount }

// OrderPlaced is a domain event. Its integration form (order.v1.placed, see
// infrastructure/messaging) is what other contexts, e.g. customer, subscribe to.
type OrderPlaced struct {
	OrderID     string
	CustomerID  string
	ProductName string
	Amount      int64 // minor units of Currency
	Currency    string
	At          time.Time
}

func (e OrderPlaced) EventName() string     { return "order.placed" }
func (e OrderPlaced) OccurredAt() time.Time { return e.At }
func (e OrderPlaced) AggregateID() string   { return e.OrderID }
