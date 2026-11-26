package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/example/myapp/internal/order/domain"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

func mustMoney(t *testing.T, amount int64, currency string) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(amount, currency)
	if err != nil {
		t.Fatalf("NewMoney(%d, %q): %v", amount, currency, err)
	}
	return m
}

func mustProduct(t *testing.T, v string) domain.ProductName {
	t.Helper()
	n, err := domain.NewProductName(v)
	if err != nil {
		t.Fatalf("NewProductName(%q): %v", v, err)
	}
	return n
}

func TestPlace_RecordsOrderPlaced(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id, err := domain.NewOrderID("o-1")
	if err != nil {
		t.Fatal(err)
	}
	o, err := domain.Place(id, "c-1", mustProduct(t, "  Coffee beans 1kg "), mustMoney(t, 1999, "EUR"), now)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if o.ID().String() != "o-1" {
		t.Fatalf("id = %s", o.ID())
	}
	if o.ProductName().String() != "Coffee beans 1kg" {
		t.Fatalf("product name = %q, want trimmed", o.ProductName())
	}
	if o.Amount().Amount() != 1999 || o.Amount().Currency() != "EUR" {
		t.Fatalf("amount = %#v", o.Amount())
	}
	evs := o.PullEvents()
	if len(evs) != 1 {
		t.Fatalf("events = %d, want 1", len(evs))
	}
	want := domain.OrderPlaced{
		OrderID: "o-1", CustomerID: "c-1", ProductName: "Coffee beans 1kg",
		Amount: 1999, Currency: "EUR", At: now,
	}
	placed, ok := evs[0].(domain.OrderPlaced)
	if !ok || placed != want || placed.AggregateID() != "o-1" || !placed.OccurredAt().Equal(now) {
		t.Fatalf("event = %#v, want %#v", evs[0], want)
	}
}

func TestPlace_Validation(t *testing.T) {
	if _, err := domain.NewOrderID(""); !errors.Is(err, shareddomain.ErrValidation) {
		t.Fatalf("NewOrderID(\"\") err = %v, want ErrValidation", err)
	}
	id, _ := domain.NewOrderID("o-1")
	price := mustMoney(t, 100, "USD")
	tea := mustProduct(t, "Tea")
	t.Run("no customer", func(t *testing.T) {
		if _, err := domain.Place(id, "", tea, price, time.Now()); !errors.Is(err, shareddomain.ErrValidation) {
			t.Fatalf("err = %v, want ErrValidation", err)
		}
	})
	for _, blank := range []string{"", "   "} {
		if _, err := domain.NewProductName(blank); !errors.Is(err, shareddomain.ErrValidation) {
			t.Fatalf("NewProductName(%q) err = %v, want ErrValidation", blank, err)
		}
	}
	t.Run("zero product name", func(t *testing.T) {
		if _, err := domain.Place(id, "c-1", domain.ProductName{}, price, time.Now()); !errors.Is(err, shareddomain.ErrValidation) {
			t.Fatalf("err = %v, want ErrValidation", err)
		}
	})
	t.Run("zero money", func(t *testing.T) {
		if _, err := domain.Place(id, "c-1", tea, domain.Money{}, time.Now()); !errors.Is(err, shareddomain.ErrValidation) {
			t.Fatalf("err = %v, want ErrValidation", err)
		}
	})
}

func TestNewMoney_Validation(t *testing.T) {
	cases := []struct {
		name     string
		amount   int64
		currency string
		ok       bool
	}{
		{"valid", 1, "EUR", true},
		{"zero amount", 0, "EUR", false},
		{"negative amount", -5, "EUR", false},
		{"empty currency", 100, "", false},
		{"lowercase currency", 100, "eur", false},
		{"two letters", 100, "EU", false},
		{"four letters", 100, "EURO", false},
		{"digits", 100, "E1R", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := domain.NewMoney(c.amount, c.currency)
			if c.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !c.ok && !errors.Is(err, shareddomain.ErrValidation) {
				t.Fatalf("err = %v, want ErrValidation", err)
			}
		})
	}
}
