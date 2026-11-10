// Package scenarios holds black-box behavior scenarios written once and run
// against every target:
//
//   - tests/integration, in-process, once per profile (memory, postgres,
//     postgres+kafka) via testsupport.NewApp;
//   - tests/e2e, against any running deployment (harness, staging).
//
// Scenarios only use the public contract (REST paths, proto clients), create
// their own uniquely named data, never delete anything, and wait for the
// eventually consistent read side with polling, so they are safe to run in
// parallel and to re-run against shared environments.
package scenarios

import (
	"testing"
	"time"

	"google.golang.org/grpc"

	customerv1 "github.com/example/myapp/api/proto/customer/v1"
	orderv1 "github.com/example/myapp/api/proto/order/v1"
	"github.com/example/myapp/internal/testsupport/api"
)

// Target is what a scenario runs against.
type Target struct {
	API *api.Client
	// Optional: gRPC scenarios skip when nil.
	Customers customerv1.CustomerServiceClient
	Orders    orderv1.OrderServiceClient
	// ReadTimeout bounds how long to wait for the read model to catch up
	// (outbox relay + bus + projection).
	ReadTimeout time.Duration
}

// Scenario is one named behavior.
type Scenario struct {
	Name string
	Run  func(t *testing.T, tg Target)
}

// All returns every scenario.
func All() []Scenario {
	return []Scenario{
		{"Health", Health},
		{"OpenAPI", OpenAPI},
		{"CustomerLifecycle", CustomerLifecycle},
		{"CustomerValidation", CustomerValidation},
		{"CustomerNotFound", CustomerNotFound},
		{"CustomerList", CustomerList},
		{"OrderREST", OrderREST},
		{"OrderGRPC", OrderGRPC},
		{"CustomerGRPC", CustomerGRPC},
	}
}

// RunAll runs every scenario as a parallel subtest.
func RunAll(t *testing.T, tg Target) {
	t.Helper()
	if tg.ReadTimeout == 0 {
		tg.ReadTimeout = 15 * time.Second
	}
	for _, s := range All() {
		t.Run(s.Name, func(t *testing.T) {
			t.Parallel()
			s.Run(t, tg)
		})
	}
}

// waitForReady makes gRPC calls ride out the server's startup race.
var waitForReady = grpc.WaitForReady(true)
