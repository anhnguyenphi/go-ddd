package kafka

import (
	"strconv"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// Kafka message headers used to carry retry bookkeeping alongside the encoded
// Event. Kept out-of-band (headers, not payload) so a handler that only cares
// about the domain payload never has to know retries exist.
const (
	headerAttempt       = "x-retry-attempt"    // attempts already made, as decimal
	headerNotBefore     = "x-retry-not-before" // RFC3339Nano; hop is not due before this
	headerOriginalTopic = "x-original-topic"   // topic the message first arrived on
	headerLastError     = "x-last-error"       // most recent handler error, for triage
)

// BackoffFunc computes the delay before retry attempt n (1-based: the delay
// before the *first* retry is BackoffFunc(1)). Swap this in Config for a
// different curve (fixed, jittered, ...) without touching the consumer loop.
type BackoffFunc func(attempt int) time.Duration

// ExponentialBackoff doubles base every attempt, capped at maxDelay.
func ExponentialBackoff(base, maxDelay time.Duration) BackoffFunc {
	return func(attempt int) time.Duration {
		if attempt < 1 {
			attempt = 1
		}
		d := base
		for i := 1; i < attempt; i++ {
			d *= 2
			if d >= maxDelay {
				return maxDelay
			}
		}
		if d > maxDelay {
			return maxDelay
		}
		return d
	}
}

// outcome is the pure retry/DLQ decision, factored out of the I/O-bound
// consumer loop so it is unit-testable without a broker.
type outcome int

const (
	outcomeRetry outcome = iota
	outcomeDeadLetter
)

// decideOutcome says what to do with a message whose handler has now failed
// `attempt` times total (1-based), given a ceiling of maxRetries retries
// (i.e. maxRetries+1 tries overall: the original try plus maxRetries hops).
func decideOutcome(attempt, maxRetries int) outcome {
	if attempt > maxRetries {
		return outcomeDeadLetter
	}
	return outcomeRetry
}

// defaultRetryTopicFor names the per-attempt retry hop topic, e.g.
// "customer.v1.events" attempt 1 -> "customer.v1.events.retry.1".
func defaultRetryTopicFor(topic string, attempt int) string {
	return topic + ".retry." + strconv.Itoa(attempt)
}

// defaultDLQTopicFor names the terminal dead-letter topic for a topic.
func defaultDLQTopicFor(topic string) string {
	return topic + ".dlq"
}

// retryHeaders builds the header set for a message being sent to a retry hop
// or the DLQ. lastErr may be nil for defensive callers, but callers on the
// failure path always have one.
func retryHeaders(attempt int, notBefore time.Time, originalTopic string, lastErr error) []kafkago.Header {
	h := []kafkago.Header{
		{Key: headerAttempt, Value: []byte(strconv.Itoa(attempt))},
		{Key: headerNotBefore, Value: []byte(notBefore.UTC().Format(time.RFC3339Nano))},
		{Key: headerOriginalTopic, Value: []byte(originalTopic)},
	}
	if lastErr != nil {
		h = append(h, kafkago.Header{Key: headerLastError, Value: []byte(lastErr.Error())})
	}
	return h
}

// headerValue returns the string value of the first header named key, or "".
func headerValue(headers []kafkago.Header, key string) string {
	for _, h := range headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// attemptFrom reads headerAttempt, defaulting to 0 (never retried yet) when
// absent or malformed.
func attemptFrom(headers []kafkago.Header) int {
	n, err := strconv.Atoi(headerValue(headers, headerAttempt))
	if err != nil {
		return 0
	}
	return n
}

// notBeforeFrom reads headerNotBefore, defaulting to the zero time (due
// immediately) when absent or malformed.
func notBeforeFrom(headers []kafkago.Header) time.Time {
	v := headerValue(headers, headerNotBefore)
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t
}
