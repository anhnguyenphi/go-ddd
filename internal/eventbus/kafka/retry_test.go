package kafka

import (
	"errors"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

func TestExponentialBackoff(t *testing.T) {
	b := ExponentialBackoff(time.Second, 30*time.Second)
	cases := map[int]time.Duration{
		0:  time.Second, // clamped up to attempt 1
		1:  time.Second,
		2:  2 * time.Second,
		3:  4 * time.Second,
		4:  8 * time.Second,
		5:  16 * time.Second,
		6:  30 * time.Second, // would be 32s, capped
		20: 30 * time.Second,
	}
	for attempt, want := range cases {
		if got := b(attempt); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", attempt, got, want)
		}
	}
}

func TestDecideOutcome(t *testing.T) {
	const maxRetries = 3
	cases := []struct {
		attempt int
		want    outcome
	}{
		{1, outcomeRetry},
		{2, outcomeRetry},
		{3, outcomeRetry}, // last allowed retry hop
		{4, outcomeDeadLetter},
		{5, outcomeDeadLetter},
	}
	for _, c := range cases {
		if got := decideOutcome(c.attempt, maxRetries); got != c.want {
			t.Errorf("decideOutcome(%d, %d) = %v, want %v", c.attempt, maxRetries, got, c.want)
		}
	}
}

func TestDefaultTopicNaming(t *testing.T) {
	if got := defaultTopicFor("customer.v1.created"); got != "customer.v1.events" {
		t.Errorf("defaultTopicFor = %q, want customer.v1.events", got)
	}
	if got := defaultTopicFor("malformed"); got != "malformed" {
		t.Errorf("defaultTopicFor(malformed) = %q, want unchanged", got)
	}
	if got := defaultRetryTopicFor("customer.v1.events", 2); got != "customer.v1.events.retry.2" {
		t.Errorf("defaultRetryTopicFor = %q", got)
	}
	if got := defaultDLQTopicFor("customer.v1.events"); got != "customer.v1.events.dlq" {
		t.Errorf("defaultDLQTopicFor = %q", got)
	}
}

func TestRetryHeadersRoundTrip(t *testing.T) {
	notBefore := time.Now().Add(5 * time.Second).UTC()
	headers := retryHeaders(2, notBefore, "customer.v1.events", errors.New("db unavailable"))

	if got := attemptFrom(headers); got != 2 {
		t.Errorf("attemptFrom = %d, want 2", got)
	}
	if got := notBeforeFrom(headers); !got.Equal(notBefore) {
		t.Errorf("notBeforeFrom = %v, want %v", got, notBefore)
	}
	if got := headerValue(headers, headerOriginalTopic); got != "customer.v1.events" {
		t.Errorf("original topic header = %q", got)
	}
	if got := headerValue(headers, headerLastError); got != "db unavailable" {
		t.Errorf("last error header = %q", got)
	}
}

func TestAttemptAndNotBeforeDefaultWhenAbsent(t *testing.T) {
	var headers []kafkago.Header
	if got := attemptFrom(headers); got != 0 {
		t.Errorf("attemptFrom(nil) = %d, want 0", got)
	}
	if got := notBeforeFrom(headers); !got.IsZero() {
		t.Errorf("notBeforeFrom(nil) = %v, want zero time", got)
	}
}
