//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/example/myapp/internal/testsupport"
	"github.com/example/myapp/internal/testsupport/containers"
	"github.com/example/myapp/internal/testsupport/scenarios"
)

// profile is one wiring of the application.
type profile struct {
	name        string
	readTimeout time.Duration
	opts        func(t *testing.T) []testsupport.Option
}

var profiles = []profile{
	{
		name:        "memory",
		readTimeout: 5 * time.Second,
		opts:        func(*testing.T) []testsupport.Option { return nil },
	},
	{
		name:        "postgres",
		readTimeout: 10 * time.Second,
		opts: func(t *testing.T) []testsupport.Option {
			return []testsupport.Option{testsupport.WithPostgres(containers.PostgresDSN(t))}
		},
	},
	{
		// The production topology: Postgres + Kafka. The first read waits for
		// the consumer group to join, hence the longer timeout.
		name:        "postgres+kafka",
		readTimeout: 60 * time.Second,
		opts: func(t *testing.T) []testsupport.Option {
			return []testsupport.Option{
				testsupport.WithPostgres(containers.PostgresDSN(t)),
				testsupport.WithKafka(containers.KafkaBrokers(t), containers.UniqueGroupID(t)),
			}
		},
	},
}

// TestScenarios runs every shared scenario against every profile. A new
// scenario in internal/testsupport/scenarios is picked up here and by
// tests/e2e automatically.
func TestScenarios(t *testing.T) {
	for _, p := range profiles {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			opts := append(p.opts(t), testsupport.WithGRPC(), testsupport.WithBackground())
			app := testsupport.NewApp(t, opts...)
			scenarios.RunAll(t, scenarios.Target{
				API:         app.API,
				Customers:   app.Customers,
				Orders:      app.Orders,
				ReadTimeout: p.readTimeout,
			})
		})
	}
}

// TestOutboxDrain_IsTheOnlyPathToTheReadModel pins the write -> outbox ->
// relay -> bus -> projection path deterministically: with no background relay,
// nothing reaches the read model until DrainOutbox runs.
func TestOutboxDrain_IsTheOnlyPathToTheReadModel(t *testing.T) {
	app := testsupport.NewApp(t)
	id, _ := app.API.CreateCustomer(t, "Drained")

	app.API.Get(t, "/api/v1/customers/"+id).Expect(t, http.StatusNotFound)

	n, err := app.DrainOutbox(context.Background())
	if err != nil || n == 0 {
		t.Fatalf("DrainOutbox n=%d err=%v", n, err)
	}
	app.API.Get(t, "/api/v1/customers/"+id).Expect(t, http.StatusOK)

	if n, err := app.DrainOutbox(context.Background()); err != nil || n != 0 {
		t.Fatalf("second DrainOutbox n=%d err=%v, want 0 (already relayed)", n, err)
	}
}
