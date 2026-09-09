package domain

import "time"

// DomainEvent is something that happened in the domain that domain experts care
// about. Domain events are internal to a bounded context; they are translated
// into versioned *integration events* at the infrastructure boundary before they
// leave the context (see each context's infrastructure/messaging package).
type DomainEvent interface {
	// EventName is a stable, human-readable identifier, e.g. "customer.created".
	EventName() string
	// OccurredAt is when the event happened, in UTC.
	OccurredAt() time.Time
	// AggregateID is the identity of the aggregate that produced the event.
	AggregateID() string
}
