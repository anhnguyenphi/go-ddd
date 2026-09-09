// Package kafka is a placeholder Kafka-backed eventbus.Bus. It compiles with no
// external dependencies and documents the integration seams; wire a real client
// (segmentio/kafka-go, confluent-kafka-go, ...) into the marked spots.
package kafka

import (
	"context"
	"errors"
	"log/slog"

	"github.com/example/myapp/internal/eventbus"
)

// Config describes how to reach the cluster and how to name topics.
type Config struct {
	Brokers []string
	// TopicFor maps an event Name to a topic. Default: bounded-context-oriented
	// topics (see PROJECT.md §9), e.g. "customer.v1.created" -> "customer.v1.events".
	TopicFor func(eventName string) string
	GroupID  string
}

// ErrNotWired is returned until a real client is plugged in.
var ErrNotWired = errors.New("kafka: client not wired — see internal/eventbus/kafka/bus.go")

// Bus is the Kafka adapter skeleton.
type Bus struct {
	cfg    Config
	logger *slog.Logger
	// writer *kafka.Writer      // TODO: real producer
	// readers map[string]...     // TODO: consumer group per subscription
}

// New constructs the adapter. It does not dial anything yet.
func New(cfg Config, logger *slog.Logger) *Bus {
	if cfg.TopicFor == nil {
		cfg.TopicFor = defaultTopicFor
	}
	return &Bus{cfg: cfg, logger: logger}
}

func defaultTopicFor(eventName string) string {
	// "customer.v1.created" -> "customer.v1.events"
	for i := 0; i < len(eventName); i++ {
		if eventName[i] == '.' {
			for j := i + 1; j < len(eventName); j++ {
				if eventName[j] == '.' {
					return eventName[:j] + ".events"
				}
			}
		}
	}
	return eventName
}

// Publish would marshal each Event and produce it to cfg.TopicFor(e.Name),
// keyed by AggregateID to preserve per-aggregate ordering.
func (b *Bus) Publish(ctx context.Context, events ...eventbus.Event) error {
	_ = ctx
	for _, e := range events {
		b.logger.Warn("kafka publish skipped (not wired)",
			"topic", b.cfg.TopicFor(e.Name), "event", e.Name, "id", e.ID)
	}
	return ErrNotWired
}

// Subscribe would start a consumer-group reader for the topic backing eventName
// and invoke handler per message, committing offsets on success.
func (b *Bus) Subscribe(eventName string, handler eventbus.Handler) error {
	_ = handler
	b.logger.Warn("kafka subscribe skipped (not wired)",
		"topic", b.cfg.TopicFor(eventName), "event", eventName)
	return ErrNotWired
}

// Close would flush the producer and stop all readers.
func (b *Bus) Close() error { return nil }
