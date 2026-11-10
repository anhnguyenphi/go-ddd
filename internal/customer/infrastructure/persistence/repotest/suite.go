// Package repotest is a conformance suite for customer domain.Repository
// implementations. Every adapter (memory, postgres, any future one) runs the
// same cases, so the in-memory profile used by fast tests behaves like
// production. Test-only: imported from _test files and tests/ only.
package repotest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/example/myapp/internal/customer/domain"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// Run runs every case against a fresh repository from newRepo.
func Run(t *testing.T, newRepo func(t *testing.T) domain.Repository) {
	t.Helper()
	cases := []struct {
		name string
		fn   func(t *testing.T, repo domain.Repository)
	}{
		{"SaveThenFindRoundTrips", saveThenFindRoundTrips},
		{"FindUnknownIsNotFound", findUnknownIsNotFound},
		{"ExistsByEmail", existsByEmail},
		{"UpdateReplacesEmail", updateReplacesEmail},
		{"StaleWriteIsConflict", staleWriteIsConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.fn(t, newRepo(t)) })
	}
}

func unique() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newCustomer(t *testing.T) *domain.Customer {
	t.Helper()
	u := unique()
	email, err := domain.NewEmail("c-" + u + "@example.test")
	if err != nil {
		t.Fatal(err)
	}
	// Microsecond precision: Postgres TIMESTAMPTZ does not keep nanoseconds.
	at := time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)
	c, err := domain.NewCustomer(domain.MustCustomerID("cust-"+u), "Ada "+u, email, at)
	if err != nil {
		t.Fatal(err)
	}
	c.PullEvents()
	return c
}

func mustEmail(t *testing.T, raw string) domain.Email {
	t.Helper()
	e, err := domain.NewEmail(raw)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func saveThenFindRoundTrips(t *testing.T, repo domain.Repository) {
	ctx := context.Background()
	c := newCustomer(t)
	if err := repo.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, c.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	want, have := c.ToSnapshot(), got.ToSnapshot()
	if !have.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", have.CreatedAt, want.CreatedAt)
	}
	have.CreatedAt, want.CreatedAt = time.Time{}, time.Time{}
	if have != want {
		t.Errorf("snapshot = %+v, want %+v", have, want)
	}
	if got.HasEvents() {
		t.Error("a loaded aggregate must not carry events")
	}
}

func findUnknownIsNotFound(t *testing.T, repo domain.Repository) {
	_, err := repo.FindByID(context.Background(), domain.MustCustomerID("missing-"+unique()))
	if !errors.Is(err, shareddomain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func existsByEmail(t *testing.T, repo domain.Repository) {
	ctx := context.Background()
	c := newCustomer(t)
	if ok, err := repo.ExistsByEmail(ctx, c.Email()); err != nil || ok {
		t.Fatalf("before save: exists=%v err=%v", ok, err)
	}
	if err := repo.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if ok, err := repo.ExistsByEmail(ctx, c.Email()); err != nil || !ok {
		t.Fatalf("after save: exists=%v err=%v", ok, err)
	}
}

func updateReplacesEmail(t *testing.T, repo domain.Repository) {
	ctx := context.Background()
	c := newCustomer(t)
	if err := repo.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	old := c.Email()
	loaded, err := repo.FindByID(ctx, c.ID())
	if err != nil {
		t.Fatal(err)
	}
	newEmail := mustEmail(t, "new-"+unique()+"@example.test")
	if err := loaded.ChangeEmail(newEmail, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, loaded); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	got, err := repo.FindByID(ctx, c.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Email().Equals(newEmail) || got.Version() != 2 {
		t.Fatalf("after update: email=%s version=%d", got.Email(), got.Version())
	}
	if ok, _ := repo.ExistsByEmail(ctx, old); ok {
		t.Error("old email still reported as taken")
	}
	if ok, _ := repo.ExistsByEmail(ctx, newEmail); !ok {
		t.Error("new email not reported as taken")
	}
}

// staleWriteIsConflict: two writers load the same version; the second save
// must lose with ErrConflict (optimistic concurrency).
func staleWriteIsConflict(t *testing.T, repo domain.Repository) {
	ctx := context.Background()
	c := newCustomer(t)
	if err := repo.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	a, err := repo.FindByID(ctx, c.ID())
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.FindByID(ctx, c.ID())
	if err != nil {
		t.Fatal(err)
	}

	if err := a.ChangeEmail(mustEmail(t, "a-"+unique()+"@example.test"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("first writer: %v", err)
	}
	if err := b.ChangeEmail(mustEmail(t, "b-"+unique()+"@example.test"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, b); !errors.Is(err, shareddomain.ErrConflict) {
		t.Fatalf("stale writer err = %v, want ErrConflict", err)
	}
}
