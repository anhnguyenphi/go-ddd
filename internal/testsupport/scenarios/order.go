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

// placeOrderBody is a valid REST PlaceOrder body for customerID.
func placeOrderBody(customerID string) map[string]any {
	return map[string]any{
		"customer_id":  customerID,
		"product_name": "Coffee beans",
		"amount":       1999,
		"currency":     "EUR",
	}
}

// OrderREST: placing an order verifies the customer synchronously through
// customer's read model (so it waits for the projection first). Returns 200,
// not 201 (see internal/order/CONTEXT.md). An order needs a product name and
// a positive amount in a 3-letter currency, or it is 400.
func OrderREST(t *testing.T, tg Target) {
	c := tg.API
	id, _ := c.CreateCustomer(t, "Buyer")
	c.AwaitCustomer(t, id, tg.ReadTimeout, nil)

	var placed struct {
		ID string `json:"id"`
	}
	c.Post(t, "/api/v1/orders", placeOrderBody(id)).Expect(t, http.StatusOK).JSON(t, &placed)
	if placed.ID == "" {
		t.Fatal("no order id")
	}

	c.Post(t, "/api/v1/orders", placeOrderBody("missing-"+api.Unique())).Expect(t, http.StatusNotFound)

	for name, mutate := range map[string]func(map[string]any){
		"no product name": func(b map[string]any) { delete(b, "product_name") },
		"zero amount":     func(b map[string]any) { b["amount"] = 0 },
		"negative amount": func(b map[string]any) { b["amount"] = -1 },
		"bad currency":    func(b map[string]any) { b["currency"] = "euro" },
	} {
		body := placeOrderBody(id)
		mutate(body)
		if got := c.Post(t, "/api/v1/orders", body); got.Status != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, got.Status)
		}
	}
}

// OrderGRPC: the same over gRPC; unknown customer is codes.NotFound, invalid
// input codes.InvalidArgument.
func OrderGRPC(t *testing.T, tg Target) {
	if tg.Orders == nil {
		t.Skip("no gRPC target")
	}
	ctx := context.Background()
	id, _ := tg.API.CreateCustomer(t, "GrpcBuyer")
	tg.API.AwaitCustomer(t, id, tg.ReadTimeout, nil)

	req := func(customerID string) *orderv1.PlaceOrderRequest {
		return &orderv1.PlaceOrderRequest{CustomerId: customerID, ProductName: "Tea", Amount: 450, Currency: "USD"}
	}
	placed, err := tg.Orders.PlaceOrder(ctx, req(id), waitForReady)
	if err != nil || placed.GetId() == "" {
		t.Fatalf("PlaceOrder = %v, %v", placed, err)
	}
	_, err = tg.Orders.PlaceOrder(ctx, req("missing-"+api.Unique()), waitForReady)
	if status.Code(err) != codes.NotFound {
		t.Fatalf("PlaceOrder unknown code = %v, want NotFound", status.Code(err))
	}
	bad := req(id)
	bad.Amount = 0
	_, err = tg.Orders.PlaceOrder(ctx, bad, waitForReady)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("PlaceOrder zero amount code = %v, want InvalidArgument", status.Code(err))
	}
}
