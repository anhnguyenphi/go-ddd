//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/eventbus/kafka"
	"github.com/example/myapp/internal/platform/config"
	"github.com/example/myapp/internal/testsupport"
	"github.com/example/myapp/internal/testsupport/api"
	"github.com/example/myapp/internal/testsupport/containers"
)

// Side effects that clients can't see through the API: the welcome email is a
// log-only sender today. Run on every profile.
func TestSideEffects(t *testing.T) {
	for _, p := range profiles {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			app := testsupport.NewApp(t, append(p.opts(t), testsupport.WithBackground(), testsupport.WithLogCapture())...)

			t.Run("CustomerCreatedSendsWelcomeEmail", func(t *testing.T) {
				_, email := app.API.CreateCustomer(t, "Welcome")
				app.Logs.Await(t, `"msg":"welcome email sent".*"to":"`+regexp.QuoteMeta(email)+`"`, p.readTimeout)
			})
		})
	}
}

// Integration events produced by another producer (not the app's outbox)
// reach every subscriber: customer.v1.created is projected and triggers the
// welcome email, and order.v1.placed (which no context publishes yet) reaches
// the customer context's inbound handler. KAFKA_TOPIC_PREFIX scopes the app to its own
// topics: this is what lets harness sandboxes share one broker.
func TestKafka_ExternallyProducedEventOnPrefixedTopics(t *testing.T) {
	t.Parallel()
	brokers := containers.KafkaBrokers(t)
	prefix := "t" + api.Unique()
	app := testsupport.NewApp(t,
		testsupport.WithPostgres(containers.PostgresDSN(t)),
		testsupport.WithKafka(brokers, containers.UniqueGroupID(t)),
		testsupport.WithConfig(func(c *config.Config) { c.Kafka.TopicPrefix = prefix }),
		testsupport.WithLogCapture(),
	)

	producer := kafka.New(kafka.Config{Brokers: brokers, TopicFor: kafka.PrefixedTopicFor(prefix)},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { _ = producer.Close() })

	id := "ext-" + api.Unique()
	email := api.UniqueEmail("produced")
	payload, err := json.Marshal(map[string]string{"id": id, "name": "Produced", "email": email})
	if err != nil {
		t.Fatal(err)
	}
	if err := producer.Publish(context.Background(), eventbus.Event{
		ID: "evt-" + api.Unique(), Name: "customer.v1.created", Source: "customer",
		AggregateID: id, OccurredAt: time.Now().UTC(), Version: 1, Payload: payload,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	got := app.API.AwaitCustomer(t, id, 60*time.Second, func(c api.Customer) bool { return c.Email == email })
	if got.Status != "active" {
		t.Errorf("projected status = %q, want active", got.Status)
	}
	app.Logs.Await(t, `"msg":"welcome email sent".*"to":"`+regexp.QuoteMeta(email)+`"`, 10*time.Second)

	orderID := "ext-order-" + api.Unique()
	if err := producer.Publish(context.Background(), eventbus.Event{
		ID: "evt-" + api.Unique(), Name: "order.v1.placed", Source: "order",
		AggregateID: orderID, OccurredAt: time.Now().UTC(), Version: 1,
		Payload: []byte(`{"id":"` + orderID + `","customer_id":"` + id + `"}`),
	}); err != nil {
		t.Fatalf("Publish order.v1.placed: %v", err)
	}
	app.Logs.Await(t, `customer context observed order\.v1\.placed.*"aggregate_id":"`+regexp.QuoteMeta(orderID)+`"`, 60*time.Second)
}
