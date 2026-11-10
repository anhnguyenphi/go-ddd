package commands_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/myapp/internal/customer/application/commands"
	"github.com/example/myapp/internal/customer/domain"
	"github.com/example/myapp/internal/customer/domain/events"
	"github.com/example/myapp/internal/customer/domain/services"
	custmemory "github.com/example/myapp/internal/customer/infrastructure/persistence/memory"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/internal/shared/infrastructure/clock"
	"github.com/example/myapp/internal/shared/infrastructure/id"
	"github.com/example/myapp/internal/shared/infrastructure/transaction"
)

var changedAt = time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)

// newChangeEmail seeds a repo with customers (id -> email) and returns the
// handler wired against it.
func newChangeEmail(t *testing.T, seed map[string]string) (*commands.ChangeCustomerEmailHandler, *custmemory.Repository, *recordingPublisher) {
	t.Helper()
	repo := custmemory.New()
	for cid, raw := range seed {
		e, err := domain.NewEmail(raw)
		if err != nil {
			t.Fatal(err)
		}
		c, err := domain.NewCustomer(domain.MustCustomerID(cid), "Seed", e, changedAt.Add(-time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		c.PullEvents()
		if err := repo.Save(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
	pub := &recordingPublisher{}
	h := commands.NewChangeCustomerEmailHandler(commands.Deps{
		Repo:       repo,
		UoW:        transaction.Noop{},
		Events:     pub,
		Clock:      clock.Fixed{At: changedAt},
		IDs:        id.Fixed{Value: "unused"},
		Uniqueness: services.NewEmailUniqueness(repo),
	})
	return h, repo, pub
}

func TestChangeEmail_PersistsAndEmitsEvent(t *testing.T) {
	h, repo, pub := newChangeEmail(t, map[string]string{"c-1": "old@example.com"})

	if _, err := h.Handle(context.Background(), commands.ChangeCustomerEmail{CustomerID: "c-1", NewEmail: "new@example.com"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got, err := repo.FindByID(context.Background(), domain.MustCustomerID("c-1"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Email().String() != "new@example.com" || got.Version() != 2 {
		t.Fatalf("stored email=%s version=%d", got.Email(), got.Version())
	}
	if len(pub.events) != 1 {
		t.Fatalf("emitted %d events, want 1", len(pub.events))
	}
	ev, ok := pub.events[0].(events.CustomerEmailChanged)
	if !ok || ev.OldEmail != "old@example.com" || ev.NewEmail != "new@example.com" || !ev.At.Equal(changedAt) {
		t.Fatalf("event = %#v", pub.events[0])
	}
}

func TestChangeEmail_SameAddressIsNoOp(t *testing.T) {
	h, repo, pub := newChangeEmail(t, map[string]string{"c-1": "same@example.com"})

	if _, err := h.Handle(context.Background(), commands.ChangeCustomerEmail{CustomerID: "c-1", NewEmail: "SAME@example.com"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(pub.events) != 0 {
		t.Fatalf("emitted %d events for an unchanged email", len(pub.events))
	}
	got, _ := repo.FindByID(context.Background(), domain.MustCustomerID("c-1"))
	if got.Version() != 1 {
		t.Fatalf("version = %d, want 1 (no write)", got.Version())
	}
}

func TestChangeEmail_Errors(t *testing.T) {
	cases := []struct {
		name string
		cmd  commands.ChangeCustomerEmail
		want error
	}{
		{"taken by another customer", commands.ChangeCustomerEmail{CustomerID: "c-1", NewEmail: "taken@example.com"}, shareddomain.ErrConflict},
		{"unknown customer", commands.ChangeCustomerEmail{CustomerID: "nope", NewEmail: "x@example.com"}, shareddomain.ErrNotFound},
		{"invalid email", commands.ChangeCustomerEmail{CustomerID: "c-1", NewEmail: "not-an-email"}, shareddomain.ErrValidation},
		{"empty id", commands.ChangeCustomerEmail{CustomerID: "", NewEmail: "x@example.com"}, shareddomain.ErrValidation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, _, pub := newChangeEmail(t, map[string]string{"c-1": "mine@example.com", "c-2": "taken@example.com"})
			_, err := h.Handle(context.Background(), c.cmd)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if len(pub.events) != 0 {
				t.Fatalf("emitted %d events on failure", len(pub.events))
			}
		})
	}
}
