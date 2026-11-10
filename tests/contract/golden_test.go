//go:build contract

package contract

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/myapp/internal/eventbus"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden.json from the current output")

// wire is the contract-relevant view of an envelope. ID is random per event
// and so not pinned (its presence is checked by the caller).
type wire struct {
	Name        string          `json:"name"`
	Source      string          `json:"source"`
	Version     int             `json:"version"`
	AggregateID string          `json:"aggregate_id"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Payload     json.RawMessage `json:"payload"`
}

func assertGolden(t *testing.T, name string, e eventbus.Event) {
	t.Helper()
	if !json.Valid(e.Payload) {
		t.Fatalf("payload is not JSON: %s", e.Payload)
	}
	got, err := json.MarshalIndent(wire{
		Name: e.Name, Source: e.Source, Version: e.Version,
		AggregateID: e.AggregateID, OccurredAt: e.OccurredAt, Payload: e.Payload,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	path := filepath.Join("testdata", name+".golden.json")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s drifted from the published contract.\n--- want (%s)\n%s\n--- got\n%s\n"+
			"Additive change? re-run with -update. Breaking? add a v2 event instead.", name, path, want, got)
	}
}
