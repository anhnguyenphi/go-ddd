package commands_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/myapp/internal/customer/application/commands"
	"github.com/example/myapp/internal/customer/domain"
	"github.com/example/myapp/internal/customer/domain/services"
	custmemory "github.com/example/myapp/internal/customer/infrastructure/persistence/memory"
	sharedapp "github.com/example/myapp/internal/shared/application"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/internal/shared/infrastructure/clock"
	"github.com/example/myapp/internal/shared/infrastructure/id"
	"github.com/example/myapp/internal/shared/infrastructure/transaction"
)

// recordingPublisher captures the domain events the handler emits.
type recordingPublisher struct{ events []shareddomain.DomainEvent }

func (p *recordingPublisher) Publish(_ context.Context, e ...shareddomain.DomainEvent) error {
	p.events = append(p.events, e...)
	return nil
}

func newHandler(t *testing.T) (*commands.CreateCustomerHandler, *custmemory.Repository, *recordingPublisher) {
	t.Helper()
	repo := custmemory.New()
	pub := &recordingPublisher{}
	d := commands.Deps{
		Repo:       repo,
		UoW:        transaction.Noop{},
		Events:     pub,
		Clock:      clock.Fixed{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		IDs:        id.Fixed{Value: "cust-1"},
		Uniqueness: services.NewEmailUniqueness(repo),
	}
	return commands.NewCreateCustomerHandler(d), repo, pub
}

func TestCreateCustomer_PersistsAndEmitsEvent(t *testing.T) {
	h, repo, pub := newHandler(t)

	res, err := h.Handle(context.Background(), commands.CreateCustomer{
		Name: "Ada", Email: "ada@example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CustomerID != "cust-1" {
		t.Fatalf("id = %q, want cust-1", res.CustomerID)
	}

	got, err := repo.FindByID(context.Background(), domain.MustCustomerID("cust-1"))
	if err != nil {
		t.Fatalf("not persisted: %v", err)
	}
	if got.Email().String() != "ada@example.com" {
		t.Fatalf("email = %q", got.Email().String())
	}
	if len(pub.events) != 1 {
		t.Fatalf("emitted %d events, want 1", len(pub.events))
	}
}

func TestCreateCustomer_RejectsInvalidEmailBeforeTouchingRepo(t *testing.T) {
	h, _, pub := newHandler(t)

	_, err := h.Handle(context.Background(), commands.CreateCustomer{Name: "Ada", Email: "nope"})
	if !errors.Is(err, shareddomain.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if len(pub.events) != 0 {
		t.Fatal("no events should be emitted on validation failure")
	}
}

func TestCreateCustomer_EnforcesEmailUniqueness(t *testing.T) {
	h, _, _ := newHandler(t)
	ctx := context.Background()

	if _, err := h.Handle(ctx, commands.CreateCustomer{Name: "Ada", Email: "dup@example.com"}); err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	_, err := h.Handle(ctx, commands.CreateCustomer{Name: "Bob", Email: "dup@example.com"})
	if !errors.Is(err, shareddomain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

var _ sharedapp.DomainEventPublisher = (*recordingPublisher)(nil)
