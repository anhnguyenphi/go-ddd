package eventbus

import "context"

// Handler consumes one integration event. Returning an error signals the bus to
// retry (or dead-letter) according to its delivery policy.
type Handler func(ctx context.Context, event Event) error

// Bus is both ends of the pipe. Most call sites depend on the narrower
// Publisher or Subscriber instead.
type Bus interface {
	Publisher
	Subscriber
}
