//go:build contract

// Package contract pins the customer context's PUBLISHED integration-event
// contract: event names and payload shapes that other services depend on.
// A change here is a breaking change and must bump the version (v1 -> v2).
//
//	go test -tags=contract ./tests/contract/...
package contract

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/example/myapp/internal/customer/domain/events"
	"github.com/example/myapp/internal/customer/infrastructure/messaging"
)

func TestCustomerCreated_ContractIsStable(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	ie, ok := messaging.ToIntegrationEvent(events.CustomerCreated{
		CustomerID: "c-1", Name: "Ada", Email: "ada@example.com", At: at,
	})
	if !ok {
		t.Fatal("CustomerCreated must map to an integration event")
	}
	if ie.Name != "customer.v1.created" {
		t.Fatalf("event name = %q, want customer.v1.created", ie.Name)
	}
	if ie.Source != "customer" || ie.Version != 1 || ie.AggregateID != "c-1" {
		t.Fatalf("envelope drift: %+v", ie)
	}

	var payload map[string]any
	if err := json.Unmarshal(ie.Payload, &payload); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	assertKeys(t, payload, "id", "name", "email")
	if payload["email"] != "ada@example.com" {
		t.Fatalf("payload.email = %v", payload["email"])
	}
}

func TestCustomerEmailChanged_ContractIsStable(t *testing.T) {
	ie, ok := messaging.ToIntegrationEvent(events.CustomerEmailChanged{
		CustomerID: "c-1", OldEmail: "a@x.com", NewEmail: "b@x.com", At: time.Now(),
	})
	if !ok || ie.Name != "customer.v1.email_changed" {
		t.Fatalf("unexpected mapping: ok=%v name=%q", ok, ie.Name)
	}
	var payload map[string]any
	_ = json.Unmarshal(ie.Payload, &payload)
	assertKeys(t, payload, "id", "old_email", "new_email")
}

func assertKeys(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	if len(m) != len(keys) {
		t.Fatalf("payload has keys %v, want exactly %v", mapKeys(m), keys)
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Fatalf("payload missing key %q (have %v)", k, mapKeys(m))
		}
	}
}

func mapKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
