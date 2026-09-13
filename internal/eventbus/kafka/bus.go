// Package kafka is a Kafka-backed eventbus.Bus. Publish produces to a topic
// derived from the event name; Subscribe starts a consumer-group reader per
// topic that decodes each message, dispatches it to every handler registered
// for its event name, and commits the offset on success.
//
// Delivery policy on handler failure is retry-topic hops: a failed message is
// republished to a per-attempt "<topic>.retry.<n>" topic carrying a
// not-before deadline, consumed by its own reader once that deadline passes.
// After Config.MaxRetries hops it lands on "<topic>.dlq" instead, and the
// original offset is committed either way so one poison message never blocks
// its partition. See retry.go for the hop/DLQ decision and wire.go for the
// message codec — both are injectable seams for extending this later (a
// different backoff curve, topic-naming scheme, or wire format).
package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/example/myapp/internal/eventbus"
)

// Config describes how to reach the cluster, how to name topics, and how to
// handle delivery failures. Every func field has a default (see New) so
// callers only set what they need to change.
type Config struct {
	Brokers []string
	GroupID string

	// TopicFor maps an event Name to its topic. Default: bounded-context
	// topics (see PROJECT.md §9), e.g. "customer.v1.created" -> "customer.v1.events".
	TopicFor func(eventName string) string

	// MaxRetries is how many retry-hop attempts a failed message gets before
	// it is dead-lettered. Default 3.
	MaxRetries int
	// Backoff computes the delay before retry attempt n (1-based). Default
	// ExponentialBackoff(1s, 30s).
	Backoff BackoffFunc
	// RetryTopicFor names the topic for retry attempt n of topic. Default
	// "<topic>.retry.<n>".
	RetryTopicFor func(topic string, attempt int) string
	// DLQTopicFor names the terminal dead-letter topic for topic. Default
	// "<topic>.dlq".
	DLQTopicFor func(topic string) string

	// Codec (de)serializes Event to the Kafka message value. Default JSONCodec.
	Codec Codec
}

func (c *Config) setDefaults() {
	if c.TopicFor == nil {
		c.TopicFor = defaultTopicFor
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
	if c.Backoff == nil {
		c.Backoff = ExponentialBackoff(time.Second, 30*time.Second)
	}
	if c.RetryTopicFor == nil {
		c.RetryTopicFor = defaultRetryTopicFor
	}
	if c.DLQTopicFor == nil {
		c.DLQTopicFor = defaultDLQTopicFor
	}
	if c.Codec == nil {
		c.Codec = JSONCodec{}
	}
	if c.GroupID == "" {
		c.GroupID = "myapp"
	}
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

// binding is one handler registered for one event name, already wrapped with
// the bus-wide middleware chain.
type binding struct {
	eventName string
	handler   eventbus.Handler
}

// Bus is the Kafka adapter. Zero value is not usable; build with New.
type Bus struct {
	cfg    Config
	logger *slog.Logger
	mw     []eventbus.Middleware

	writer *kafkago.Writer

	mu       sync.RWMutex
	bindings map[string][]binding // business topic -> handlers
	started  bool
	readers  []*kafkago.Reader
	wg       sync.WaitGroup
}

var _ eventbus.Bus = (*Bus)(nil)

// New constructs the adapter and dials the producer side. It does not start
// consuming until Run is called. mw wraps every handler, same as local.New.
func New(cfg Config, logger *slog.Logger, mw ...eventbus.Middleware) *Bus {
	cfg.setDefaults()
	writer := &kafkago.Writer{
		Addr:                   kafkago.TCP(cfg.Brokers...),
		Balancer:               &kafkago.Hash{}, // key = AggregateID: same aggregate, same partition, in order
		RequiredAcks:           kafkago.RequireAll,
		AllowAutoTopicCreation: true,
		Logger:                 slogPrintf{logger: logger, level: slog.LevelDebug},
		ErrorLogger:            slogPrintf{logger: logger, level: slog.LevelError},
	}
	return &Bus{
		cfg:      cfg,
		logger:   logger,
		mw:       mw,
		writer:   writer,
		bindings: make(map[string][]binding),
	}
}

// Subscribe registers handler for eventName. Must be called before Run: Run
// snapshots the current bindings to decide which topics to consume and does
// not pick up subscriptions registered afterward.
func (b *Bus) Subscribe(eventName string, handler eventbus.Handler) error {
	if eventName == "" || handler == nil {
		return errors.New("kafka: eventName and handler are required")
	}
	topic := b.cfg.TopicFor(eventName)
	wrapped := eventbus.Chain(handler, b.mw...)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.bindings[topic] = append(b.bindings[topic], binding{eventName: eventName, handler: wrapped})
	b.logger.Debug("event subscription registered", "event", eventName, "topic", topic)
	return nil
}

// Publish produces each event to its topic, keyed by AggregateID.
func (b *Bus) Publish(ctx context.Context, events ...eventbus.Event) error {
	if len(events) == 0 {
		return nil
	}
	msgs := make([]kafkago.Message, 0, len(events))
	for _, e := range events {
		payload, err := b.cfg.Codec.Encode(e)
		if err != nil {
			return err
		}
		msgs = append(msgs, kafkago.Message{
			Topic: b.cfg.TopicFor(e.Name),
			Key:   []byte(e.AggregateID),
			Value: payload,
			Headers: []kafkago.Header{
				{Key: "x-event-name", Value: []byte(e.Name)},
			},
		})
	}
	if err := b.writer.WriteMessages(ctx, msgs...); err != nil {
		return fmt.Errorf("kafka: publish: %w", err)
	}
	return nil
}

// Run starts one consumer-group reader per subscribed topic (plus one per
// retry hop) and blocks until ctx is cancelled. The API/worker processes run
// this alongside the outbox relay (see bootstrap.Application.RunBackground).
func (b *Bus) Run(ctx context.Context) error {
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return errors.New("kafka: Run already called")
	}
	b.started = true
	topics := make([]string, 0, len(b.bindings))
	for topic := range b.bindings {
		topics = append(topics, topic)
	}
	b.mu.Unlock()

	if len(topics) == 0 {
		b.logger.Info("kafka bus: no subscriptions, nothing to consume")
	}

	for _, topic := range topics {
		b.startConsumer(ctx, topic, topic, 0) // main topic: attempt 0 (never retried)
		for attempt := 1; attempt <= b.cfg.MaxRetries; attempt++ {
			retryTopic := b.cfg.RetryTopicFor(topic, attempt)
			b.startConsumer(ctx, retryTopic, topic, attempt)
		}
	}

	<-ctx.Done()
	b.logger.Info("kafka bus: stopping")
	b.mu.RLock()
	readers := append([]*kafkago.Reader(nil), b.readers...)
	b.mu.RUnlock()
	for _, r := range readers {
		_ = r.Close() // unblocks any in-flight FetchMessage
	}
	b.wg.Wait()
	return ctx.Err()
}

// startConsumer launches the read loop for one physical topic (either the
// business topic itself, hopAttempt==0, or one of its retry hops).
func (b *Bus) startConsumer(ctx context.Context, physicalTopic, businessTopic string, _ int) {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:  b.cfg.Brokers,
		GroupID:  b.cfg.GroupID,
		Topic:    physicalTopic,
		MinBytes: 1,
		MaxBytes: 10e6,
		Logger:   slogPrintf{logger: b.logger, level: slog.LevelDebug},
	})

	b.mu.Lock()
	b.readers = append(b.readers, reader)
	b.mu.Unlock()

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		c := &consumer{bus: b, reader: reader, businessTopic: businessTopic, physicalTopic: physicalTopic}
		c.run(ctx)
	}()
}

// Close releases the producer and any consumers started by Run.
func (b *Bus) Close() error {
	var errs []error
	if err := b.writer.Close(); err != nil {
		errs = append(errs, err)
	}
	b.mu.RLock()
	readers := append([]*kafkago.Reader(nil), b.readers...)
	b.mu.RUnlock()
	for _, r := range readers {
		if err := r.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// handlersFor returns the bindings for a business topic whose eventName
// matches e.Name.
func (b *Bus) handlersFor(businessTopic, eventName string) []eventbus.Handler {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []eventbus.Handler
	for _, bd := range b.bindings[businessTopic] {
		if bd.eventName == eventName {
			out = append(out, bd.handler)
		}
	}
	return out
}

// slogPrintf adapts *slog.Logger to kafka-go's Logger interface (Printf).
type slogPrintf struct {
	logger *slog.Logger
	level  slog.Level
}

func (l slogPrintf) Printf(format string, args ...interface{}) {
	l.logger.Log(context.Background(), l.level, fmt.Sprintf(format, args...))
}
