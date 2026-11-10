package kafka

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/example/myapp/internal/eventbus"
)

func newTestBus() *Bus {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(Config{}, logger)
}

func TestSubscribeGroupsHandlersByTopic(t *testing.T) {
	b := newTestBus()
	var created, emailChanged int
	if err := b.Subscribe("customer.v1.created", func(context.Context, eventbus.Event) error {
		created++
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := b.Subscribe("customer.v1.email_changed", func(context.Context, eventbus.Event) error {
		emailChanged++
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Both event names map to the same context topic (default TopicFor), so
	// Run would start exactly one main-topic reader for them, and dispatch
	// must still separate handlers by event name.
	handlers := b.handlersFor("customer.v1.events", "customer.v1.created")
	if len(handlers) != 1 {
		t.Fatalf("handlersFor(created) = %d handlers, want 1", len(handlers))
	}
	if err := handlers[0](context.Background(), eventbus.Event{}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if created != 1 || emailChanged != 0 {
		t.Fatalf("created=%d emailChanged=%d, want 1/0", created, emailChanged)
	}
}

func TestSubscribeRequiresEventNameAndHandler(t *testing.T) {
	b := newTestBus()
	if err := b.Subscribe("", func(context.Context, eventbus.Event) error { return nil }); err == nil {
		t.Error("Subscribe with empty eventName: want error, got nil")
	}
	if err := b.Subscribe("x", nil); err == nil {
		t.Error("Subscribe with nil handler: want error, got nil")
	}
}

func TestMiddlewareWrapsSubscribedHandlers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var mwRan bool
	mw := func(next eventbus.Handler) eventbus.Handler {
		return func(ctx context.Context, e eventbus.Event) error {
			mwRan = true
			return next(ctx, e)
		}
	}
	b := New(Config{}, logger, mw)
	_ = b.Subscribe("customer.v1.created", func(context.Context, eventbus.Event) error { return nil })

	handlers := b.handlersFor("customer.v1.events", "customer.v1.created")
	if len(handlers) != 1 {
		t.Fatalf("handlersFor = %d handlers, want 1", len(handlers))
	}
	if err := handlers[0](context.Background(), eventbus.Event{}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !mwRan {
		t.Error("middleware did not run around the subscribed handler")
	}
}

func TestPrefixedTopicFor(t *testing.T) {
	cases := []struct{ prefix, event, want string }{
		{"", "customer.v1.created", "customer.v1.events"},
		{"sb_feature_a", "customer.v1.created", "sb_feature_a.customer.v1.events"},
		{"sb_x", "order.v1.placed", "sb_x.order.v1.events"},
	}
	for _, c := range cases {
		if got := PrefixedTopicFor(c.prefix)(c.event); got != c.want {
			t.Errorf("PrefixedTopicFor(%q)(%q) = %q, want %q", c.prefix, c.event, got, c.want)
		}
	}
}

func TestPrefixedTopicsDeriveRetryAndDLQ(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := New(Config{TopicFor: PrefixedTopicFor("sb_x"), MaxRetries: 1}, logger)
	topic := b.cfg.TopicFor("customer.v1.created")
	if got := b.cfg.RetryTopicFor(topic, 1); got != "sb_x.customer.v1.events.retry.1" {
		t.Errorf("retry topic = %q", got)
	}
	if got := b.cfg.DLQTopicFor(topic); got != "sb_x.customer.v1.events.dlq" {
		t.Errorf("dlq topic = %q", got)
	}
}
