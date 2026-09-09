// Package eventbus defines a transport-agnostic integration-event bus plus a few
// implementations (in-process, Kafka stub) and the transactional outbox that
// makes publication reliable.
//
// Integration events crossing this bus are the *public* contract of a bounded
// context. They are versioned (customer.v1.created) and decoupled from the
// internal domain event that triggered them.
package eventbus

import "time"

// Event is a self-describing envelope. Payload is the serialized event body
// (JSON in this skeleton); everything else is metadata used for routing,
// tracing, idempotency, and ordering.
type Event struct {
	ID            string            // unique per event instance (dedupe key)
	Name          string            // versioned type, e.g. "customer.v1.created"
	Source        string            // producing context, e.g. "customer"
	AggregateID   string            // entity the event is about
	OccurredAt    time.Time         // domain time, UTC
	Version       int               // schema version of Payload
	CorrelationID string            // ties together one business transaction
	CausationID   string            // the event/command that caused this one
	Payload       []byte            // serialized body
	Metadata      map[string]string // free-form (tenant, actor, ...)
}
