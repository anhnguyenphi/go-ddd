//go:build contract

package contract

import (
	"testing"

	"github.com/example/myapp/internal/order/domain"
	"github.com/example/myapp/internal/order/infrastructure/messaging"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

func TestOrderEvents_MatchGolden(t *testing.T) {
	cases := []struct {
		golden string
		event  shareddomain.DomainEvent
	}{
		{"order.v1.placed", domain.OrderPlaced{
			OrderID: "o-1", CustomerID: "c-1", ProductName: "Coffee beans",
			Amount: 1999, Currency: "EUR", At: at,
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
