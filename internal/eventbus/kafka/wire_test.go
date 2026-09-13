package kafka

import (
	"testing"
	"time"

	"github.com/example/myapp/internal/eventbus"
)

func TestJSONCodecRoundTrip(t *testing.T) {
	want := eventbus.Event{
		ID:            "evt-1",
		Name:          "customer.v1.created",
		Source:        "customer",
		AggregateID:   "cust-1",
		OccurredAt:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Version:       1,
		CorrelationID: "corr-1",
		CausationID:   "cmd-1",
		Payload:       []byte(`{"email":"a@b.com"}`),
		Metadata:      map[string]string{"tenant": "acme"},
	}

	b, err := (JSONCodec{}).Encode(want)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := (JSONCodec{}).Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.ID != want.ID || got.Name != want.Name || got.AggregateID != want.AggregateID ||
		!got.OccurredAt.Equal(want.OccurredAt) || got.Version != want.Version ||
		got.CorrelationID != want.CorrelationID || got.CausationID != want.CausationID ||
		string(got.Payload) != string(want.Payload) || got.Metadata["tenant"] != "acme" {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestJSONCodecDecodeInvalid(t *testing.T) {
	if _, err := (JSONCodec{}).Decode([]byte("not json")); err == nil {
		t.Fatal("Decode of invalid JSON: want error, got nil")
	}
}
