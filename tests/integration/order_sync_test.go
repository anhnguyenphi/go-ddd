//go:build integration

// This file exercises the *synchronous* counterpart to
// customer_flow_test.go's async outbox -> read-model flow: order.PlaceOrder
// calls the real, wired customer context in-process (see
// internal/order/infrastructure/customerclient) and gets an answer back
// before it does anything else — no bus, no outbox, no eventual consistency
// on order's side. It goes over the real order.v1.OrderService contract, both
// gRPC directly (internal/order/interfaces/grpc) and REST (grpc-gateway
// transcoding the same contract — see order/module.go's RegisterGateway),
// the same way an external caller would use either.
package integration

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	orderv1 "github.com/example/myapp/api/proto/order/v1"
	"github.com/example/myapp/internal/bootstrap"
	"github.com/example/myapp/internal/platform/config"
)

// newOrderApp wires the full application, like newApp, but also brings its
// gRPC server up on an ephemeral local port and returns a client dialed
// against it, for the tests that want to hit order.v1.OrderService directly
// rather than through its REST transcoding.
func newOrderApp(t *testing.T) (*bootstrap.Application, *httptest.Server, orderv1.OrderServiceClient) {
	t.Helper()
	cfg := config.Default()
	cfg.Log.Level = "error"
	cfg.GRPC.Addr = freeAddr(t)

	app, err := bootstrap.Build(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	httpSrv := httptest.NewServer(app.HTTPHandler())

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = app.GRPCServer().Run(runCtx)
		close(done)
	}()

	conn, err := grpc.NewClient(cfg.GRPC.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", cfg.GRPC.Addr, err)
	}

	t.Cleanup(func() {
		_ = conn.Close()
		httpSrv.Close()
		cancel()
		<-done
		_ = app.Close()
	})

	return app, httpSrv, orderv1.NewOrderServiceClient(conn)
}

// freeAddr asks the OS for a free TCP port and immediately releases it. There
// is a small, accepted race between the release and grpcx.Server binding it —
// the same trade-off any "find a free port for a test server" helper makes.
func freeAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeAddr: %v", err)
	}
	addr := lis.Addr().String()
	lis.Close()
	return addr
}

func TestPlaceOrder_SynchronouslyVerifiesCustomerAcrossContexts(t *testing.T) {
	app, httpSrv, orders := newOrderApp(t)
	ctx := context.Background()

	res, err := httpSrv.Client().Post(httpSrv.URL+"/api/v1/customers", "application/json",
		strings.NewReader(`{"name":"Ada Lovelace","email":"ada@example.com"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	var created struct{ ID string }
	_ = json.NewDecoder(res.Body).Decode(&created)
	res.Body.Close()
	if created.ID == "" {
		t.Fatal("no customer id returned")
	}

	// GetCustomer, which the verifier calls, is served from customer's CQRS
	// read model — fed by the outbox relay, which only runs when
	// RunBackground does (not started by this test). This drain stands in
	// for that background loop so the projection has caught up; it is a
	// test-harness detail, not something PlaceOrder itself waits on (it
	// makes exactly one synchronous call and gets one answer, no bus
	// involved).
	if _, err := app.DrainOutbox(ctx); err != nil {
		t.Fatalf("DrainOutbox: %v", err)
	}

	// WaitForReady covers the small startup race between dialing and the
	// gRPC listener actually accepting connections.
	placed, err := orders.PlaceOrder(ctx, &orderv1.PlaceOrderRequest{CustomerId: created.ID}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("PlaceOrder for a real customer: %v", err)
	}
	if placed.GetId() == "" {
		t.Fatal("no order id returned")
	}
}

func TestPlaceOrder_RejectsUnknownCustomer(t *testing.T) {
	_, _, orders := newOrderApp(t)

	_, err := orders.PlaceOrder(context.Background(), &orderv1.PlaceOrderRequest{CustomerId: "does-not-exist"}, grpc.WaitForReady(true))
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound (err=%v)", status.Code(err), err)
	}
}

// TestPlaceOrder_ViaREST proves the grpc-gateway transcoding actually works —
// unlike the tests above, it never touches order.v1.OrderService's gRPC port
// at all, only the HTTP handler customer's own REST calls already go through.
func TestPlaceOrder_ViaREST(t *testing.T) {
	app, httpSrv, _ := newOrderApp(t)
	ctx := context.Background()

	res, err := httpSrv.Client().Post(httpSrv.URL+"/api/v1/customers", "application/json",
		strings.NewReader(`{"name":"Grace Hopper","email":"grace@example.com"}`))
	if err != nil {
		t.Fatalf("POST customer: %v", err)
	}
	var created struct{ ID string }
	_ = json.NewDecoder(res.Body).Decode(&created)
	res.Body.Close()
	if created.ID == "" {
		t.Fatal("no customer id returned")
	}

	if _, err := app.DrainOutbox(ctx); err != nil {
		t.Fatalf("DrainOutbox: %v", err)
	}

	res, err = httpSrv.Client().Post(httpSrv.URL+"/api/v1/orders", "application/json",
		strings.NewReader(`{"customer_id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("POST order: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var placed struct{ ID string }
	_ = json.NewDecoder(res.Body).Decode(&placed)
	if placed.ID == "" {
		t.Fatal("no order id returned")
	}
}
