// Package domain is the payment context's domain layer (skeleton).
package domain

import (
	"time"

	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// PaymentID identifies a Payment aggregate.
type PaymentID struct{ value string }

// NewPaymentID validates a raw id.
func NewPaymentID(v string) (PaymentID, error) {
	if v == "" {
		return PaymentID{}, shareddomain.Invalid("payment id must not be empty")
	}
	return PaymentID{value: v}, nil
}

// String returns the raw id.
func (id PaymentID) String() string { return id.value }

// Payment is the aggregate root (skeleton).
type Payment struct {
	shareddomain.AggregateRoot
	id          PaymentID
	orderID     string
	amountCents int64
	currency    string
	completedAt time.Time
}

// PaymentCompleted is a domain event.
type PaymentCompleted struct {
	PaymentID string
	OrderID   string
	At        time.Time
}

func (e PaymentCompleted) EventName() string     { return "payment.completed" }
func (e PaymentCompleted) OccurredAt() time.Time { return e.At }
func (e PaymentCompleted) AggregateID() string   { return e.PaymentID }
