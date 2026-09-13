package kafka

import (
	"context"
	"errors"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/example/myapp/internal/eventbus"
)

// consumer runs the read loop for one reader: fetch, wait out any retry
// not-before deadline, decode, dispatch to matching handlers, then either
// commit (success), hop to the next retry topic, or dead-letter. The source
// offset is committed only once the outcome has been durably recorded
// somewhere (a handler ran, or the message reached a retry/DLQ topic) — never
// before, so a crash mid-handling redelivers rather than loses the message.
type consumer struct {
	bus           *Bus
	reader        *kafkago.Reader
	businessTopic string // the topic handlers are registered against
	physicalTopic string // the topic this reader actually reads (== businessTopic for hopAttempt 0)
}

func (c *consumer) run(ctx context.Context) {
	log := c.bus.logger.With("topic", c.physicalTopic, "group", c.bus.cfg.GroupID)
	log.Info("kafka consumer started")
	defer log.Info("kafka consumer stopped")

	for {
		m, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, kafkago.ErrGroupClosed) {
				return
			}
			log.ErrorContext(ctx, "kafka fetch failed", "error", err)
			continue
		}
		c.handle(ctx, m)
	}
}

func (c *consumer) handle(ctx context.Context, m kafkago.Message) {
	log := c.bus.logger.With("topic", c.physicalTopic)

	if notBefore := notBeforeFrom(m.Headers); !notBefore.IsZero() {
		if wait := time.Until(notBefore); wait > 0 {
			t := time.NewTimer(wait)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}

	e, err := c.bus.cfg.Codec.Decode(m.Value)
	if err != nil {
		// Poison message: it will never decode, so retrying buys nothing.
		// Go straight to the DLQ.
		log.ErrorContext(ctx, "kafka message failed to decode, sending to dead-letter topic", "error", err)
		if c.deadLetter(ctx, m, err) == nil {
			c.commit(ctx, m)
		}
		return
	}

	handlers := c.bus.handlersFor(c.businessTopic, e.Name)
	if len(handlers) == 0 {
		log.DebugContext(ctx, "no subscribers for event on this node", "event", e.Name)
		c.commit(ctx, m)
		return
	}

	var errs []error
	for _, h := range handlers {
		if err := h(ctx, e); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		c.fail(ctx, m, e, err)
		return
	}
	c.commit(ctx, m)
}

// fail decides between another retry hop and the DLQ and publishes
// accordingly. The source offset is committed only if that publish
// succeeded; if it failed, the offset is left alone so the message (and the
// retry/DLQ decision) is redelivered and retried.
func (c *consumer) fail(ctx context.Context, m kafkago.Message, e eventbus.Event, handlerErr error) {
	log := c.bus.logger.With("topic", c.physicalTopic, "event", e.Name, "event_id", e.ID)
	attempt := attemptFrom(m.Headers) + 1

	switch decideOutcome(attempt, c.bus.cfg.MaxRetries) {
	case outcomeRetry:
		delay := c.bus.cfg.Backoff(attempt)
		nextTopic := c.bus.cfg.RetryTopicFor(c.businessTopic, attempt)
		notBefore := time.Now().Add(delay)
		msg := kafkago.Message{
			Topic:   nextTopic,
			Key:     m.Key,
			Value:   m.Value,
			Headers: retryHeaders(attempt, notBefore, c.businessTopic, handlerErr),
		}
		if err := c.bus.writer.WriteMessages(ctx, msg); err != nil {
			log.ErrorContext(ctx, "failed to schedule retry, leaving offset uncommitted for redelivery",
				"attempt", attempt, "error", err)
			return
		}
		log.WarnContext(ctx, "event handler failed, scheduled for retry",
			"attempt", attempt, "max_retries", c.bus.cfg.MaxRetries,
			"delay", delay.String(), "retry_topic", nextTopic, "handler_error", handlerErr)
		c.commit(ctx, m)

	case outcomeDeadLetter:
		if c.deadLetter(ctx, m, handlerErr) != nil {
			return
		}
		log.ErrorContext(ctx, "event handler exhausted retries, sent to dead-letter topic",
			"attempts", attempt, "max_retries", c.bus.cfg.MaxRetries, "handler_error", handlerErr)
		c.commit(ctx, m)
	}
}

// deadLetter writes m to its business topic's DLQ, carrying cause and the
// attempt count for triage. Returns a non-nil error if the write failed.
func (c *consumer) deadLetter(ctx context.Context, m kafkago.Message, cause error) error {
	dlqTopic := c.bus.cfg.DLQTopicFor(c.businessTopic)
	attempt := attemptFrom(m.Headers) + 1
	msg := kafkago.Message{
		Topic:   dlqTopic,
		Key:     m.Key,
		Value:   m.Value,
		Headers: retryHeaders(attempt, time.Now(), c.businessTopic, cause),
	}
	if err := c.bus.writer.WriteMessages(ctx, msg); err != nil {
		c.bus.logger.ErrorContext(ctx, "failed to write to dead-letter topic, leaving offset uncommitted",
			"topic", c.physicalTopic, "dlq_topic", dlqTopic, "error", err)
		return err
	}
	return nil
}

func (c *consumer) commit(ctx context.Context, m kafkago.Message) {
	if err := c.reader.CommitMessages(ctx, m); err != nil && ctx.Err() == nil {
		c.bus.logger.ErrorContext(ctx, "kafka offset commit failed", "topic", c.physicalTopic, "error", err)
	}
}
