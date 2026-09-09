package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/example/myapp/internal/customer/domain"
	"github.com/example/myapp/internal/customer/domain/events"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

func mustEmail(t *testing.T, raw string) domain.Email {
	t.Helper()
	e, err := domain.NewEmail(raw)
	if err != nil {
		t.Fatalf("NewEmail(%q): %v", raw, err)
	}
	return e
}

func TestNewCustomer_RecordsCreatedEvent(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	c, err := domain.NewCustomer(domain.MustCustomerID("c-1"), "Ada", mustEmail(t, "ada@example.com"), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Status() != domain.StatusActive {
		t.Fatalf("status = %s, want active", c.Status())
	}

	evs := c.PullEvents()
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1", len(evs))
	}
	created, ok := evs[0].(events.CustomerCreated)
	if !ok {
		t.Fatalf("event type = %T, want CustomerCreated", evs[0])
	}
	if created.Email != "ada@example.com" || created.AggregateID() != "c-1" {
		t.Fatalf("unexpected event: %+v", created)
	}
	if c.HasEvents() {
		t.Fatal("PullEvents should have drained the buffer")
	}
}

func TestNewCustomer_RejectsEmptyName(t *testing.T) {
	_, err := domain.NewCustomer(domain.MustCustomerID("c-1"), "", mustEmail(t, "a@b.com"), time.Now())
	if !errors.Is(err, shareddomain.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

func TestChangeEmail_NoOpWhenUnchanged(t *testing.T) {
	c, _ := domain.NewCustomer(domain.MustCustomerID("c-1"), "Ada", mustEmail(t, "ada@example.com"), time.Now())
	_ = c.PullEvents()

	if err := c.ChangeEmail(mustEmail(t, "ADA@example.com"), time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.HasEvents() {
		t.Fatal("changing to the same (normalised) address must not record an event")
	}
}

func TestChangeEmail_RecordsEventAndBumpsVersion(t *testing.T) {
	c, _ := domain.NewCustomer(domain.MustCustomerID("c-1"), "Ada", mustEmail(t, "ada@example.com"), time.Now())
	_ = c.PullEvents()
	startVersion := c.Version()

	if err := c.ChangeEmail(mustEmail(t, "ada@new.com"), time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Version() != startVersion+1 {
		t.Fatalf("version = %d, want %d", c.Version(), startVersion+1)
	}
	evs := c.PullEvents()
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1", len(evs))
	}
	if _, ok := evs[0].(events.CustomerEmailChanged); !ok {
		t.Fatalf("event type = %T, want CustomerEmailChanged", evs[0])
	}
}

func TestEmail_Validation(t *testing.T) {
	for _, bad := range []string{"", "  ", "no-at", "a@b", "@b.com", "a@", "a@@b.com"} {
		if _, err := domain.NewEmail(bad); err == nil {
			t.Errorf("NewEmail(%q) = nil error, want rejection", bad)
		}
	}
	e, err := domain.NewEmail("  Ada@Example.COM ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.String() != "ada@example.com" {
		t.Fatalf("normalised = %q, want ada@example.com", e.String())
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c, _ := domain.NewCustomer(domain.MustCustomerID("c-9"), "Grace", mustEmail(t, "grace@example.com"), now)

	got, err := domain.FromSnapshot(c.ToSnapshot())
	if err != nil {
		t.Fatalf("FromSnapshot: %v", err)
	}
	if got.ID() != c.ID() || !got.Email().Equals(c.Email()) || got.Version() != c.Version() {
		t.Fatalf("round trip mismatch: %+v vs %+v", got.ToSnapshot(), c.ToSnapshot())
	}
}
