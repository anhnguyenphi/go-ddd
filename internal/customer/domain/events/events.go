// Package events holds the customer context's *domain* events. They are plain,
// immutable facts phrased in the past tense. They never leave the context in
// this form — infrastructure/messaging maps them to versioned integration
// events first.
package events

import "time"

// CustomerCreated is raised when a new customer is registered.
type CustomerCreated struct {
	CustomerID string
	Name       string
	Email      string
	At         time.Time
}

func (e CustomerCreated) EventName() string     { return "customer.created" }
func (e CustomerCreated) OccurredAt() time.Time { return e.At }
func (e CustomerCreated) AggregateID() string   { return e.CustomerID }

// CustomerEmailChanged is raised when a customer's email address changes.
type CustomerEmailChanged struct {
	CustomerID string
	OldEmail   string
	NewEmail   string
	At         time.Time
}

func (e CustomerEmailChanged) EventName() string     { return "customer.email_changed" }
func (e CustomerEmailChanged) OccurredAt() time.Time { return e.At }
func (e CustomerEmailChanged) AggregateID() string   { return e.CustomerID }

// CustomerDeactivated is raised when a customer is deactivated.
type CustomerDeactivated struct {
	CustomerID string
	Reason     string
	At         time.Time
}

func (e CustomerDeactivated) EventName() string     { return "customer.deactivated" }
func (e CustomerDeactivated) OccurredAt() time.Time { return e.At }
func (e CustomerDeactivated) AggregateID() string   { return e.CustomerID }
