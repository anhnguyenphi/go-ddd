package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/eventbus/outbox"
)

type capturingPublisher struct {
	got  []eventbus.Event
	fail bool
}

func (p *capturingPublisher) Publish(_ context.Context, events ...eventbus.Event) error {
	if p.fail {
		return errors.New("downstream down")
	}
	p.got = append(p.got, events...)
	return nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRelay_DrainsPendingToPublisherAndMarksThem(t *testing.T) {
	store := outbox.NewMemoryStore()
	pub := &capturingPublisher{}
	relay := outbox.NewRelay(store, pub, discardLogger(), outbox.Options{})

	// The app publishes through outbox.Publisher, which only writes to the store
	// (inside the caller's transaction, in production).
	appPub := outbox.NewPublisher(store)
	_ = appPub.Publish(context.Background(),
		eventbus.Event{ID: "e1", Name: "customer.v1.created"},
		eventbus.Event{ID: "e2", Name: "customer.v1.created"},
	)
	if store.PendingCount() != 2 {
		t.Fatalf("pending = %d, want 2", store.PendingCount())
	}

	n, err := relay.Drain(context.Background())
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != 2 || len(pub.got) != 2 {
		t.Fatalf("delivered n=%d captured=%d, want 2/2", n, len(pub.got))
	}
	if store.PendingCount() != 0 {
		t.Fatalf("pending after drain = %d, want 0", store.PendingCount())
	}

	// Idempotency: a second drain has nothing to do.
	if n, _ := relay.Drain(context.Background()); n != 0 {
		t.Fatalf("second drain delivered %d, want 0", n)
	}
}

func TestRelay_LeavesMessagePendingWhenPublishFails(t *testing.T) {
	store := outbox.NewMemoryStore()
	pub := &capturingPublisher{fail: true}
	relay := outbox.NewRelay(store, pub, discardLogger(), outbox.Options{})

	_ = outbox.NewPublisher(store).Publish(context.Background(),
		eventbus.Event{ID: "e1", Name: "customer.v1.created"})

	if _, err := relay.Drain(context.Background()); err != nil {
		t.Fatalf("drain returned error: %v", err)
	}
	if store.PendingCount() != 1 {
		t.Fatalf("pending = %d, want 1 (failed publish must be retried)", store.PendingCount())
	}
}

func TestMemoryStore_DeduplicatesByEventID(t *testing.T) {
	store := outbox.NewMemoryStore()
	e := eventbus.Event{ID: "dup", Name: "customer.v1.created"}
	_ = store.Add(context.Background(), e)
	_ = store.Add(context.Background(), e)
	if store.PendingCount() != 1 {
		t.Fatalf("pending = %d, want 1", store.PendingCount())
	}
}
