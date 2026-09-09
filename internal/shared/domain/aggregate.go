package domain

// AggregateRoot is embedded by every aggregate. It buffers domain events raised
// while handling a command; the application layer pulls them after the aggregate
// is persisted and hands them to the DomainEventPublisher (which, in this
// skeleton, writes them to the transactional outbox).
type AggregateRoot struct {
	events []DomainEvent
}

// RecordEvent buffers a domain event. Call it from aggregate behaviour methods
// after the state transition has been validated and applied.
func (a *AggregateRoot) RecordEvent(event DomainEvent) {
	a.events = append(a.events, event)
}

// PullEvents returns the buffered events and clears the buffer. It is the
// application layer's responsibility to call this exactly once per unit of work.
func (a *AggregateRoot) PullEvents() []DomainEvent {
	events := a.events
	a.events = nil
	return events
}

// HasEvents reports whether there are un-pulled events.
func (a *AggregateRoot) HasEvents() bool { return len(a.events) > 0 }
