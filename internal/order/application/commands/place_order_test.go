package commands_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/myapp/internal/order/application/commands"
	"github.com/example/myapp/internal/order/domain"
	ordmemory "github.com/example/myapp/internal/order/infrastructure/persistence/memory"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/internal/shared/infrastructure/clock"
	"github.com/example/myapp/internal/shared/infrastructure/id"
)

// fakeCustomerVerifier stands in for the real customerclient.Verifier (a
// synchronous gRPC call) so this test stays a fast, dependency-free unit
// test. known holds the customer ids that "exist".
type fakeCustomerVerifier struct{ known map[string]bool }

func (f fakeCustomerVerifier) Verify(_ context.Context, customerID string) error {
	if f.known[customerID] {
		return nil
	}
	return shareddomain.ErrNotFound
}

func newHandler(t *testing.T, known ...string) (*commands.PlaceOrderHandler, *ordmemory.Repository) {
	t.Helper()
	repo := ordmemory.New()
	set := make(map[string]bool, len(known))
	for _, k := range known {
		set[k] = true
	}
	h := commands.NewPlaceOrderHandler(commands.Deps{
		Repo:     repo,
		Clock:    clock.Fixed{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		IDs:      id.Fixed{Value: "order-1"},
		Customer: fakeCustomerVerifier{known: set},
	})
	return h, repo
}

func TestPlaceOrder_VerifiesCustomerBeforePlacing(t *testing.T) {
	h, repo := newHandler(t, "cust-1")

	res, err := h.Handle(context.Background(), commands.PlaceOrder{CustomerID: "cust-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OrderID != "order-1" {
		t.Fatalf("id = %q, want order-1", res.OrderID)
	}

	if _, err := repo.FindByID(context.Background(), mustOrderID(t, "order-1")); err != nil {
		t.Fatalf("not persisted: %v", err)
	}
}

func TestPlaceOrder_RejectsUnknownCustomerWithoutPersisting(t *testing.T) {
	h, repo := newHandler(t) // no known customers

	_, err := h.Handle(context.Background(), commands.PlaceOrder{CustomerID: "ghost"})
	if !errors.Is(err, shareddomain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByID(context.Background(), mustOrderID(t, "order-1")); !errors.Is(err, shareddomain.ErrNotFound) {
		t.Fatal("order must not be persisted when the customer check fails")
	}
}

func mustOrderID(t *testing.T, v string) domain.OrderID {
	t.Helper()
	id, err := domain.NewOrderID(v)
	if err != nil {
		t.Fatalf("NewOrderID: %v", err)
	}
	return id
}
