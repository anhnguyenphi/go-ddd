package outbox_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/eventbus/outbox"
)

type syncPublisher struct {
	mu  sync.Mutex
	got []eventbus.Event
}

func (p *syncPublisher) Publish(_ context.Context, events ...eventbus.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.got = append(p.got, events...)
	return nil
}

func (p *syncPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.got)
}

func TestRelayRun_PollsUntilCancelled(t *testing.T) {
	store := outbox.NewMemoryStore()
	pub := &syncPublisher{}
	relay := outbox.NewRelay(store, pub, discardLogger(), outbox.Options{Interval: 10 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- relay.Run(ctx) }()

	_ = outbox.NewPublisher(store).Publish(context.Background(), eventbus.Event{ID: "e1", Name: "a.v1.b"})

	deadline := time.Now().Add(2 * time.Second)
	for pub.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if pub.count() != 1 {
		t.Fatalf("published %d, want 1 (relay did not poll)", pub.count())
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
	if store.PendingCount() != 0 {
		t.Fatalf("pending = %d, want 0", store.PendingCount())
	}
}
