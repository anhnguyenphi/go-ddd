//go:build integration

package kafka_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/eventbus/kafka"
	"github.com/example/myapp/internal/testsupport/api"
	"github.com/example/myapp/internal/testsupport/containers"
)

// TestMain terminates the containers this package's tests started.
func TestMain(m *testing.M) { os.Exit(containers.RunMain(m)) }

// newIsolatedBus builds a Kafka bus whose topics carry a unique prefix
// (kafka.PrefixedTopicFor, the same mapping KAFKA_TOPIC_PREFIX uses), so poison
// messages and offsets never leak into other tests on the shared broker.
func newIsolatedBus(t *testing.T) (*kafka.Bus, func(eventName string) string) {
	t.Helper()
	topicFor := kafka.PrefixedTopicFor("t" + api.Unique())
	bus := kafka.New(kafka.Config{
		Brokers:    containers.KafkaBrokers(t),
		GroupID:    containers.UniqueGroupID(t),
		TopicFor:   topicFor,
		MaxRetries: 1,
		Backoff:    kafka.ExponentialBackoff(100*time.Millisecond, 100*time.Millisecond),
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return bus, topicFor
}

func runBus(t *testing.T, bus *kafka.Bus) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- bus.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("bus.Run: %v", err)
		}
		_ = bus.Close()
	})
}

// An event published by a separate producer (another deployment, or a
// producer-only bus with no consumer group) on the same prefixed topics reaches
// the subscriber with every field intact.
func TestKafkaBus_DeliversEventIntact(t *testing.T) {
	t.Parallel()
	bus, topicFor := newIsolatedBus(t)
	producer := kafka.New(kafka.Config{Brokers: containers.KafkaBrokers(t), TopicFor: topicFor},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { _ = producer.Close() })
	got := make(chan eventbus.Event, 1)
	if err := bus.Subscribe("test.v1.happened", func(_ context.Context, e eventbus.Event) error {
		got <- e
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	runBus(t, bus)

	sent := eventbus.Event{
		ID:          "evt-" + api.Unique(),
		Name:        "test.v1.happened",
		Source:      "test",
		AggregateID: "agg-1",
		OccurredAt:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Version:     1,
		Payload:     []byte(`{"hello":"world"}`),
	}
	if err := producer.Publish(context.Background(), sent); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case e := <-got:
		if e.ID != sent.ID || e.Name != sent.Name || e.AggregateID != sent.AggregateID ||
			e.Source != sent.Source || !e.OccurredAt.Equal(sent.OccurredAt) || string(e.Payload) != string(sent.Payload) {
			t.Fatalf("received %+v, sent %+v", e, sent)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("event not delivered within 60s")
	}
}

// A handler that keeps failing sends the message through one retry hop to the
// DLQ, and the partition keeps flowing: a later message for the same key
// (same partition) is still delivered.
func TestKafkaBus_FailingHandlerRetriesThenDeadLetters(t *testing.T) {
	t.Parallel()
	bus, topicFor := newIsolatedBus(t)

	var mu sync.Mutex
	attempts := map[string]int{}
	delivered := make(chan string, 1)
	if err := bus.Subscribe("test.v1.happened", func(_ context.Context, e eventbus.Event) error {
		mu.Lock()
		attempts[e.ID]++
		mu.Unlock()
		if e.ID == "poison" {
			return errors.New("always fails")
		}
		delivered <- e.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	runBus(t, bus)

	for _, id := range []string{"poison", "healthy"} {
		if err := bus.Publish(context.Background(), eventbus.Event{
			ID: id, Name: "test.v1.happened", AggregateID: "same-key", OccurredAt: time.Now().UTC(), Payload: []byte(`{}`),
		}); err != nil {
			t.Fatalf("Publish %s: %v", id, err)
		}
	}

	select {
	case id := <-delivered:
		if id != "healthy" {
			t.Fatalf("delivered %s, want healthy", id)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("healthy message blocked behind the poison one")
	}

	// The poison message lands on <topic>.dlq with triage headers.
	dlq := topicFor("test.v1.happened") + ".dlq"
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: containers.KafkaBrokers(t),
		GroupID: containers.UniqueGroupID(t),
		Topic:   dlq,
	})
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	m, err := r.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("read %s: %v", dlq, err)
	}
	headers := map[string]string{}
	for _, h := range m.Headers {
		headers[h.Key] = string(h.Value)
	}
	if string(m.Key) != "same-key" || headers["x-retry-attempt"] != "2" || headers["x-last-error"] == "" {
		t.Fatalf("DLQ message key=%s headers=%v", m.Key, headers)
	}

	mu.Lock()
	defer mu.Unlock()
	if attempts["poison"] != 2 {
		t.Fatalf("poison handled %d times, want 2 (original + 1 retry hop)", attempts["poison"])
	}
}
