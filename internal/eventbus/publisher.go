package eventbus

import "context"

// Publisher sends integration events. The transactional outbox implements this
// interface by writing rows inside the caller's DB transaction; the relay then
// re-publishes through the "real" Publisher.
type Publisher interface {
	Publish(ctx context.Context, events ...Event) error
}

// PublisherFunc adapts a function to Publisher.
type PublisherFunc func(ctx context.Context, events ...Event) error

func (f PublisherFunc) Publish(ctx context.Context, events ...Event) error {
	return f(ctx, events...)
}

// Nop discards everything. Handy in tests and for contexts that only consume.
type Nop struct{}

func (Nop) Publish(context.Context, ...Event) error { return nil }
