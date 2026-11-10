package containers

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/redpanda"
)

var rp struct {
	once   sync.Once
	err    error
	broker string
}

func startRedpanda() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ctr, err := redpanda.Run(ctx, RedpandaImage, redpanda.WithAutoCreateTopics())
	if ctr != nil {
		onTerminate(ctr)
	}
	if err != nil {
		rp.err = fmt.Errorf("start redpanda: %w", err)
		return
	}
	rp.broker, rp.err = ctr.KafkaSeedBroker(ctx)
}

// KafkaBrokers returns the seed brokers of a shared Redpanda (Kafka API)
// container with topic auto-creation on. Topics are shared between the tests
// of one binary, so isolate with UniqueGroupID, unique aggregate ids, and, for
// tests that publish poison messages, a unique topic prefix.
func KafkaBrokers(t testing.TB) []string {
	t.Helper()
	Require(t)
	rp.once.Do(startRedpanda)
	if rp.err != nil {
		unavailable(t, "redpanda", rp.err)
	}
	return []string{rp.broker}
}

// UniqueGroupID returns a consumer group no other test uses.
func UniqueGroupID(t testing.TB) string {
	t.Helper()
	return fmt.Sprintf("test-%s-%d", sanitize(t.Name()), time.Now().UnixNano())
}

func sanitize(s string) string {
	out := []byte(s)
	for i, c := range out {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			out[i] = '-'
		}
	}
	return string(out)
}
