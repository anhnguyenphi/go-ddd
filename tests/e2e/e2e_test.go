//go:build e2e

// Package e2e runs the shared black-box scenarios (internal/testsupport/
// scenarios) against an already running deployment: the local harness,
// docker-compose, or staging. It only speaks the public contract (REST + the
// api/proto gRPC clients), creates uniquely named data and never deletes
// anything, so it is safe to re-run against shared environments. tests/arch
// enforces that it imports nothing from the application itself.
//
//	E2E_BASE_URL=http://localhost:58080 E2E_GRPC_ADDR=localhost:59090 \
//	  go test -tags=e2e ./tests/e2e/...
//
// E2E_BASE_URL is required (the suite skips without it). E2E_GRPC_ADDR is
// optional; gRPC scenarios skip without it. E2E_READ_TIMEOUT (a Go duration,
// default 30s) bounds waits for the eventually consistent read side.
package e2e

import (
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	customerv1 "github.com/example/myapp/api/proto/customer/v1"
	orderv1 "github.com/example/myapp/api/proto/order/v1"
	"github.com/example/myapp/internal/testsupport/api"
	"github.com/example/myapp/internal/testsupport/scenarios"
)

func target(t *testing.T) scenarios.Target {
	t.Helper()
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	tg := scenarios.Target{API: api.NewClient(base), ReadTimeout: 30 * time.Second}

	if v := os.Getenv("E2E_READ_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("E2E_READ_TIMEOUT: %v", err)
		}
		tg.ReadTimeout = d
	}

	if addr := os.Getenv("E2E_GRPC_ADDR"); addr != "" {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("dial %s: %v", addr, err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		tg.Customers = customerv1.NewCustomerServiceClient(conn)
		tg.Orders = orderv1.NewOrderServiceClient(conn)
	}
	return tg
}

func TestScenarios(t *testing.T) {
	scenarios.RunAll(t, target(t))
}
