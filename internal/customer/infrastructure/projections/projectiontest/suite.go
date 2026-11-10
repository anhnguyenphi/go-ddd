// Package projectiontest is a conformance suite for the customer read model
// (projections.ReadModel). It drives each implementation the way production
// does: integration events from messaging.ToIntegrationEvent, delivered by a
// bus to the handlers the read model registers. Test-only.
package projectiontest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/example/myapp/internal/customer/domain/events"
	"github.com/example/myapp/internal/customer/infrastructure/messaging"
	"github.com/example/myapp/internal/customer/infrastructure/projections"
	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/eventbus/local"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/pkg/pagination"
)

// Run runs every case against a fresh, empty read model from newModel.
func Run(t *testing.T, newModel func(t *testing.T) projections.ReadModel) {
	t.Helper()
	cases := []struct {
		name string
		fn   func(t *testing.T, h *harness)
	}{
		{"CreatedIsVisible", createdIsVisible},
		{"UnknownIsNotFound", unknownIsNotFound},
		{"EmailChangedUpdatesEmail", emailChangedUpdatesEmail},
		{"DeactivatedMarksInactive", deactivatedMarksInactive},
		{"RedeliveredCreatedIsIdempotent", redeliveredCreatedIsIdempotent},
		{"ListPaginatesInCreationOrder", listPaginatesInCreationOrder},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newModel(t)
			bus := local.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err := m.Register(bus); err != nil {
				t.Fatalf("Register: %v", err)
			}
			c.fn(t, &harness{model: m, bus: bus})
		})
	}
}

type harness struct {
	model projections.ReadModel
	bus   eventbus.Bus
}

// deliver maps domain events to integration events and publishes them, the
// same path the outbox relay takes.
func (h *harness) deliver(t *testing.T, evs ...shareddomain.DomainEvent) {
	t.Helper()
	for _, de := range evs {
		ie, ok := messaging.ToIntegrationEvent(de)
		if !ok {
			t.Fatalf("no integration event for %T", de)
		}
		if err := h.bus.Publish(context.Background(), ie); err != nil {
			t.Fatalf("publish %s: %v", ie.Name, err)
		}
	}
}

func unique() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// base is microsecond-precise so Postgres round-trips it exactly.
var base = time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)

func created(id string, at time.Time) events.CustomerCreated {
	return events.CustomerCreated{CustomerID: id, Name: "Name " + id, Email: id + "@example.test", At: at}
}

func get(t *testing.T, h *harness, id string) (status, email string) {
	t.Helper()
	c, err := h.model.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID(%s): %v", id, err)
	}
	return c.Status, c.Email
}

func createdIsVisible(t *testing.T, h *harness) {
	id := "c-" + unique()
	h.deliver(t, created(id, base))

	c, err := h.model.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if c.ID != id || c.Name != "Name "+id || c.Email != id+"@example.test" || c.Status != "active" {
		t.Fatalf("row = %+v", c)
	}
	if !c.CreatedAt.Equal(base) {
		t.Fatalf("CreatedAt = %v, want %v (the event's OccurredAt)", c.CreatedAt, base)
	}
}

func unknownIsNotFound(t *testing.T, h *harness) {
	_, err := h.model.GetByID(context.Background(), "missing-"+unique())
	if !errors.Is(err, shareddomain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func emailChangedUpdatesEmail(t *testing.T, h *harness) {
	id := "c-" + unique()
	h.deliver(t,
		created(id, base),
		events.CustomerEmailChanged{CustomerID: id, OldEmail: id + "@example.test", NewEmail: "new-" + id + "@example.test", At: base},
	)
	if _, email := get(t, h, id); email != "new-"+id+"@example.test" {
		t.Fatalf("email = %s", email)
	}
}

func deactivatedMarksInactive(t *testing.T, h *harness) {
	id := "c-" + unique()
	h.deliver(t, created(id, base), events.CustomerDeactivated{CustomerID: id, Reason: "test", At: base})
	if status, _ := get(t, h, id); status != "inactive" {
		t.Fatalf("status = %s, want inactive", status)
	}
}

// Delivery is at-least-once: a created event may arrive again after later
// events. It must not resurrect a deactivated customer or add a row.
func redeliveredCreatedIsIdempotent(t *testing.T, h *harness) {
	id := "c-" + unique()
	h.deliver(t,
		created(id, base),
		events.CustomerDeactivated{CustomerID: id, Reason: "test", At: base},
		created(id, base),
	)
	if status, _ := get(t, h, id); status != "inactive" {
		t.Fatalf("status = %s after redelivered created, want inactive", status)
	}
	res, err := h.model.List(context.Background(), pagination.New(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("total = %d, want 1", res.Total)
	}
}

func listPaginatesInCreationOrder(t *testing.T, h *harness) {
	u := unique()
	// Delivered out of creation order on purpose; c and d share a timestamp
	// so the id tie-break is exercised.
	ids := []string{"c-" + u + "-d", "c-" + u + "-a", "c-" + u + "-c", "c-" + u + "-b"}
	ats := []time.Time{base.Add(2 * time.Second), base, base.Add(2 * time.Second), base.Add(time.Second)}
	for i := range ids {
		h.deliver(t, created(ids[i], ats[i]))
	}
	want := []string{"c-" + u + "-a", "c-" + u + "-b", "c-" + u + "-c", "c-" + u + "-d"}

	first, err := h.model.List(context.Background(), pagination.New(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.model.List(context.Background(), pagination.New(2, 2))
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 4 || second.Total != 4 {
		t.Fatalf("total = %d/%d, want 4", first.Total, second.Total)
	}
	var got []string
	for _, c := range append(first.Items, second.Items...) {
		got = append(got, c.ID)
	}
	if len(got) != 4 {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
