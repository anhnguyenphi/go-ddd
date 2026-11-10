package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/example/myapp/internal/order/domain"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

func TestPlace_RecordsOrderPlaced(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id, err := domain.NewOrderID("o-1")
	if err != nil {
		t.Fatal(err)
	}
	o, err := domain.Place(id, "c-1", now)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if o.ID().String() != "o-1" {
		t.Fatalf("id = %s", o.ID())
	}
	evs := o.PullEvents()
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1", len(evs))
	}
	placed, ok := evs[0].(domain.OrderPlaced)
	if !ok || placed.CustomerID != "c-1" || placed.AggregateID() != "o-1" || !placed.OccurredAt().Equal(now) {
		t.Fatalf("event = %#v", evs[0])
	}
}

func TestPlace_Validation(t *testing.T) {
	if _, err := domain.NewOrderID(""); !errors.Is(err, shareddomain.ErrValidation) {
		t.Fatalf("NewOrderID(\"\") err = %v, want ErrValidation", err)
	}
	id, _ := domain.NewOrderID("o-1")
	if _, err := domain.Place(id, "", time.Now()); !errors.Is(err, shareddomain.ErrValidation) {
		t.Fatalf("Place without customer err = %v, want ErrValidation", err)
	}
}
