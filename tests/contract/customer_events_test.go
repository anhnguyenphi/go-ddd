//go:build contract

// Package contract pins the PUBLISHED integration-event contract: event names,
// envelope fields and exact payload shapes that other services depend on.
// Each event's wire form is compared against a golden file in testdata/, so a
// change shows up as a reviewable diff.
//
//	go test -tags=contract ./tests/contract/...
//	go test -tags=contract ./tests/contract/... -update   # rewrite goldens
//
// Only ADDITIVE changes may be accepted with -update (a new optional field).
// Renaming, removing or retyping a field is breaking: publish a new event
// version (customer.v2.*) alongside v1 instead.
package contract

import (
	"testing"
	"time"

	"github.com/example/myapp/internal/customer/domain/events"
	"github.com/example/myapp/internal/customer/infrastructure/messaging"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

var at = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func TestCustomerEvents_MatchGolden(t *testing.T) {
	cases := []struct {
		golden string
		event  shareddomain.DomainEvent
	}{
		{"customer.v1.created", events.CustomerCreated{
			CustomerID: "c-1", Name: "Ada", Email: "ada@example.com", At: at,
		}},
		{"customer.v1.email_changed", events.CustomerEmailChanged{
			CustomerID: "c-1", OldEmail: "a@example.com", NewEmail: "b@example.com", At: at,
		}},
		{"customer.v1.deactivated", events.CustomerDeactivated{
			CustomerID: "c-1", Reason: "requested", At: at,
		}},
	}
	for _, c := range cases {
		t.Run(c.golden, func(t *testing.T) {
			ie, ok := messaging.ToIntegrationEvent(c.event)
			if !ok {
				t.Fatalf("%T must map to an integration event", c.event)
			}
			if ie.Name != c.golden {
				t.Fatalf("event name = %q, want %q", ie.Name, c.golden)
			}
			if ie.ID == "" {
				t.Fatal("envelope ID (the consumer dedupe key) is empty")
			}
			assertGolden(t, c.golden, ie)
		})
	}
}
