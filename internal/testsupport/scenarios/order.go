package scenarios

import (
	"context"
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/example/myapp/api/proto/order/v1"
	"github.com/example/myapp/internal/testsupport/api"
)

// OrderREST: placing an order verifies the customer synchronously through
// customer's read model (so it waits for the projection first). Returns 200,
// not 201 (see docs/agent/learnings.md).
func OrderREST(t *testing.T, tg Target) {
	c := tg.API
	id, _ := c.CreateCustomer(t, "Buyer")
	c.AwaitCustomer(t, id, tg.ReadTimeout, nil)

	var placed struct {
		ID string `json:"id"`
	}
	c.Post(t, "/api/v1/orders", map[string]string{"customer_id": id}).Expect(t, http.StatusOK).JSON(t, &placed)
	if placed.ID == "" {
		t.Fatal("no order id")
	}

	c.Post(t, "/api/v1/orders", map[string]string{"customer_id": "missing-" + api.Unique()}).Expect(t, http.StatusNotFound)
}

// OrderGRPC: the same over gRPC; unknown customer is codes.NotFound.
func OrderGRPC(t *testing.T, tg Target) {
	if tg.Orders == nil {
		t.Skip("no gRPC target")
	}
	ctx := context.Background()
	id, _ := tg.API.CreateCustomer(t, "GrpcBuyer")
	tg.API.AwaitCustomer(t, id, tg.ReadTimeout, nil)

	placed, err := tg.Orders.PlaceOrder(ctx, &orderv1.PlaceOrderRequest{CustomerId: id}, waitForReady)
	if err != nil || placed.GetId() == "" {
		t.Fatalf("PlaceOrder = %v, %v", placed, err)
	}
	_, err = tg.Orders.PlaceOrder(ctx, &orderv1.PlaceOrderRequest{CustomerId: "missing-" + api.Unique()}, waitForReady)
	if status.Code(err) != codes.NotFound {
		t.Fatalf("PlaceOrder unknown code = %v, want NotFound", status.Code(err))
	}
}
