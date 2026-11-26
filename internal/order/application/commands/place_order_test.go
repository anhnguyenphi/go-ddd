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

var placedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// fakeCustomerVerifier stands in for the real customerclient.Verifier (a
// synchronous gRPC call) so this test stays a fast, dependency-free unit
// test. known holds the customer ids that "exist".
type fakeCustomerVerifier struct {
	known map[string]bool
	calls *int
}

func (f fakeCustomerVerifier) Verify(_ context.Context, customerID string) error {
	*f.calls++
	if f.known[customerID] {
		return nil
	}
	return shareddomain.ErrNotFound
}

// recordingUoW runs fn and remembers whether anything ran inside it.
type recordingUoW struct{ calls int }

type uowKey struct{}

func (u *recordingUoW) Within(ctx context.Context, fn func(ctx context.Context) error) error {
	u.calls++
	return fn(context.WithValue(ctx, uowKey{}, true))
}

// recordingPublisher captures published domain events and whether each
// publish happened inside the unit of work.
type recordingPublisher struct {
	events   []shareddomain.DomainEvent
	inUoW    []bool
	failWith error
}

func (p *recordingPublisher) Publish(ctx context.Context, evs ...shareddomain.DomainEvent) error {
	if p.failWith != nil {
		return p.failWith
	}
	inside, _ := ctx.Value(uowKey{}).(bool)
	for range evs {
		p.inUoW = append(p.inUoW, inside)
	}
	p.events = append(p.events, evs...)
	return nil
}

type fixture struct {
	h           *commands.PlaceOrderHandler
	verifyCalls *int
	repo        *ordmemory.Repository
	uow         *recordingUoW
	pub         *recordingPublisher
}

func newFixture(t *testing.T, known ...string) fixture {
	t.Helper()
	f := fixture{repo: ordmemory.New(), uow: &recordingUoW{}, pub: &recordingPublisher{}, verifyCalls: new(int)}
	set := make(map[string]bool, len(known))
	for _, k := range known {
		set[k] = true
	}
	f.h = commands.NewPlaceOrderHandler(commands.Deps{
		Repo:     f.repo,
		UoW:      f.uow,
		Events:   f.pub,
		Clock:    clock.Fixed{At: placedAt},
		IDs:      id.Fixed{Value: "order-1"},
		Customer: fakeCustomerVerifier{known: set, calls: f.verifyCalls},
	})
	return f
}

func validCmd(customerID string) commands.PlaceOrder {
	return commands.PlaceOrder{CustomerID: customerID, ProductName: "Coffee beans", Amount: 1999, Currency: "EUR"}
}

func TestPlaceOrder_VerifiesCustomerBeforePlacing(t *testing.T) {
	f := newFixture(t, "cust-1")

	res, err := f.h.Handle(context.Background(), validCmd("cust-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OrderID != "order-1" {
		t.Fatalf("id = %q, want order-1", res.OrderID)
	}

	o, err := f.repo.FindByID(context.Background(), mustOrderID(t, "order-1"))
	if err != nil {
		t.Fatalf("not persisted: %v", err)
	}
	if o.ProductName().String() != "Coffee beans" || o.Amount().Amount() != 1999 || o.Amount().Currency() != "EUR" {
		t.Fatalf("persisted order = %q %d %s", o.ProductName().String(), o.Amount().Amount(), o.Amount().Currency())
	}
}

func TestPlaceOrder_PublishesOrderPlacedInsideUnitOfWork(t *testing.T) {
	f := newFixture(t, "cust-1")

	if _, err := f.h.Handle(context.Background(), validCmd("cust-1")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.uow.calls != 1 {
		t.Fatalf("UoW.Within calls = %d, want 1", f.uow.calls)
	}
	if len(f.pub.events) != 1 {
		t.Fatalf("published %d events, want 1", len(f.pub.events))
	}
	want := domain.OrderPlaced{
		OrderID: "order-1", CustomerID: "cust-1", ProductName: "Coffee beans",
		Amount: 1999, Currency: "EUR", At: placedAt,
	}
	if got, ok := f.pub.events[0].(domain.OrderPlaced); !ok || got != want {
		t.Fatalf("event = %#v, want %#v", f.pub.events[0], want)
	}
	if !f.pub.inUoW[0] {
		t.Fatal("OrderPlaced must be published inside UoW.Within")
	}
}

func TestPlaceOrder_RejectsUnknownCustomerWithoutPersistingOrPublishing(t *testing.T) {
	f := newFixture(t) // no known customers

	_, err := f.h.Handle(context.Background(), validCmd("ghost"))
	if !errors.Is(err, shareddomain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := f.repo.FindByID(context.Background(), mustOrderID(t, "order-1")); !errors.Is(err, shareddomain.ErrNotFound) {
		t.Fatal("order must not be persisted when the customer check fails")
	}
	if len(f.pub.events) != 0 {
		t.Fatalf("published %d events, want 0", len(f.pub.events))
	}
}

func TestPlaceOrder_RejectsInvalidProductOrAmountWithoutPersistingOrPublishing(t *testing.T) {
	cases := map[string]func(*commands.PlaceOrder){
		"blank product name": func(c *commands.PlaceOrder) { c.ProductName = " " },
		"zero amount":        func(c *commands.PlaceOrder) { c.Amount = 0 },
		"negative amount":    func(c *commands.PlaceOrder) { c.Amount = -1 },
		"bad currency":       func(c *commands.PlaceOrder) { c.Currency = "euro" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, "cust-1")
			cmd := validCmd("cust-1")
			mutate(&cmd)

			_, err := f.h.Handle(context.Background(), cmd)
			if !errors.Is(err, shareddomain.ErrValidation) {
				t.Fatalf("err = %v, want ErrValidation", err)
			}
			if _, err := f.repo.FindByID(context.Background(), mustOrderID(t, "order-1")); !errors.Is(err, shareddomain.ErrNotFound) {
				t.Fatal("order must not be persisted on invalid input")
			}
			if len(f.pub.events) != 0 {
				t.Fatalf("published %d events, want 0", len(f.pub.events))
			}
			if *f.verifyCalls != 0 {
				t.Fatalf("customer verified %d times, want 0: invalid input must not cost a cross-context call", *f.verifyCalls)
			}
		})
	}
}

func TestPlaceOrder_InvalidInputForUnknownCustomerIsValidationNotNotFound(t *testing.T) {
	f := newFixture(t) // no known customers
	cmd := validCmd("ghost")
	cmd.ProductName = ""

	if _, err := f.h.Handle(context.Background(), cmd); !errors.Is(err, shareddomain.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestPlaceOrder_PublishFailureFailsTheCommand(t *testing.T) {
	f := newFixture(t, "cust-1")
	boom := errors.New("outbox down")
	f.pub.failWith = boom

	if _, err := f.h.Handle(context.Background(), validCmd("cust-1")); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	// Known gap, pinned on purpose (internal/order/CONTEXT.md, Open questions):
	// the order repository does not enlist in the unit of work, so the order
	// stays stored although its OrderPlaced was never published. When the
	// repository becomes transactional, flip this assertion.
	if _, err := f.repo.FindByID(context.Background(), mustOrderID(t, "order-1")); err != nil {
		t.Fatalf("today the order stays stored after a failed publish; got %v", err)
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
