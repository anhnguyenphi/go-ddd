//go:build e2e

// Package e2e drives a already-running deployment over HTTP. Point it at an
// environment and run:
//
//	E2E_BASE_URL=http://localhost:8080 go test -tags=e2e ./tests/e2e/...
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func baseURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("E2E_BASE_URL")
	if u == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	return strings.TrimRight(u, "/")
}

func TestHealthAndCustomerLifecycle(t *testing.T) {
	base := baseURL(t)
	client := &http.Client{Timeout: 10 * time.Second}

	// health
	if r, err := client.Get(base + "/healthz"); err != nil || r.StatusCode != 200 {
		t.Fatalf("healthz: err=%v status=%v", err, statusOf(r))
	}

	// create
	email := fmt.Sprintf("e2e+%d@example.com", time.Now().UnixNano())
	r, err := client.Post(base+"/api/v1/customers", "application/json",
		strings.NewReader(`{"name":"E2E","email":"`+email+`"}`))
	if err != nil || r.StatusCode != http.StatusCreated {
		t.Fatalf("create: err=%v status=%v", err, statusOf(r))
	}
	var created struct{ ID string }
	_ = json.NewDecoder(r.Body).Decode(&created)
	r.Body.Close()

	// the read model is eventually consistent — poll briefly
	deadline := time.Now().Add(5 * time.Second)
	for {
		g, err := client.Get(base + "/api/v1/customers/" + created.ID)
		if err == nil && g.StatusCode == http.StatusOK {
			g.Body.Close()
			return
		}
		if g != nil {
			g.Body.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("customer %s never appeared in the read model", created.ID)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func statusOf(r *http.Response) any {
	if r == nil {
		return "<nil>"
	}
	return r.StatusCode
}
