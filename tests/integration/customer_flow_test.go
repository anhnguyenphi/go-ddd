//go:build integration

// Package integration wires the whole application graph (bootstrap.Build) and
// exercises a use case end to end through the real HTTP handler, event bus,
// transactional outbox and read-model projection — but still with the in-memory
// profile, so it needs no external infrastructure.
//
//	go test -tags=integration ./tests/integration/...
package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/myapp/internal/bootstrap"
	"github.com/example/myapp/internal/platform/config"
)

func newApp(t *testing.T) (*bootstrap.Application, *httptest.Server) {
	t.Helper()
	cfg := config.Default()
	cfg.Log.Level = "error"

	app, err := bootstrap.Build(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	srv := httptest.NewServer(app.HTTPHandler())
	t.Cleanup(func() {
		srv.Close()
		_ = app.Close()
	})
	return app, srv
}

func TestCreateCustomer_PropagatesThroughOutboxToReadModel(t *testing.T) {
	app, srv := newApp(t)
	ctx := context.Background()

	// 1. command side
	res, err := http.Post(srv.URL+"/api/v1/customers", "application/json",
		strings.NewReader(`{"name":"Ada Lovelace","email":"ada@example.com"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	var created struct{ ID string }
	_ = json.NewDecoder(res.Body).Decode(&created)
	res.Body.Close()
	if created.ID == "" {
		t.Fatal("no id returned")
	}

	// 2. drive the relay: outbox -> bus -> projection
	if n, err := app.DrainOutbox(ctx); err != nil || n == 0 {
		t.Fatalf("DrainOutbox n=%d err=%v", n, err)
	}

	// 3. query side reflects it
	got, err := http.Get(srv.URL + "/api/v1/customers/" + created.ID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (projection not updated?)", got.StatusCode)
	}
	var customer struct {
		ID, Name, Email, Status string
	}
	_ = json.NewDecoder(got.Body).Decode(&customer)
	if customer.Email != "ada@example.com" || customer.Status != "active" {
		t.Fatalf("unexpected read model: %+v", customer)
	}
}

func TestCreateCustomer_DuplicateEmailIsRejected(t *testing.T) {
	_, srv := newApp(t)
	body := `{"name":"A","email":"dup@example.com"}`

	first, _ := http.Post(srv.URL+"/api/v1/customers", "application/json", strings.NewReader(body))
	first.Body.Close()
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first create status = %d", first.StatusCode)
	}

	second, _ := http.Post(srv.URL+"/api/v1/customers", "application/json", strings.NewReader(body))
	second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("second create status = %d, want 409", second.StatusCode)
	}
}
