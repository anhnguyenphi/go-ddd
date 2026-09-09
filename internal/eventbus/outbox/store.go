// Package outbox implements the transactional outbox pattern: integration events
// are written to a store *inside the same transaction* as the state change, then
// a relay forwards them to the real bus at-least-once.
package outbox

import (
	"context"
	"sync"
	"time"

	"github.com/example/myapp/internal/eventbus"
)

// Status is the lifecycle of an outbox row.
type Status string

const (
	StatusPending   Status = "pending"
	StatusPublished Status = "published"
	StatusFailed    Status = "failed"
)

// Message is a persisted, not-yet-delivered integration event.
type Message struct {
	Event       eventbus.Event
	Status      Status
	Attempts    int
	CreatedAt   time.Time
	PublishedAt *time.Time
	LastError   string
}

// Store persists and dispenses outbox messages.
//
// Add MUST participate in the ambient transaction (see
// shared/infrastructure/transaction). FetchPending / Mark* run outside it, on
// the relay's own connection.
type Store interface {
	Add(ctx context.Context, events ...eventbus.Event) error
	FetchPending(ctx context.Context, limit int) ([]Message, error)
	MarkPublished(ctx context.Context, eventIDs ...string) error
	MarkFailed(ctx context.Context, eventID, reason string) error
}

// MemoryStore is a goroutine-safe in-memory Store for the default wiring and
// tests. It keeps insertion order.
type MemoryStore struct {
	mu    sync.Mutex
	order []string
	byID  map[string]*Message
}

// NewMemoryStore builds an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byID: make(map[string]*Message)}
}

func (s *MemoryStore) Add(_ context.Context, events ...eventbus.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range events {
		if _, dup := s.byID[e.ID]; dup {
			continue
		}
		s.byID[e.ID] = &Message{Event: e, Status: StatusPending, CreatedAt: time.Now().UTC()}
		s.order = append(s.order, e.ID)
	}
	return nil
}

func (s *MemoryStore) FetchPending(_ context.Context, limit int) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Message
	for _, gid := range s.order {
		m := s.byID[gid]
		if m.Status == StatusPublished {
			continue
		}
		out = append(out, *m)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *MemoryStore) MarkPublished(_ context.Context, eventIDs ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for _, gid := range eventIDs {
		if m, ok := s.byID[gid]; ok {
			m.Status = StatusPublished
			m.PublishedAt = &now
			m.LastError = ""
		}
	}
	return nil
}

func (s *MemoryStore) MarkFailed(_ context.Context, eventID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.byID[eventID]; ok {
		m.Status = StatusFailed
		m.Attempts++
		m.LastError = reason
	}
	return nil
}

// PendingCount is a test/inspection helper.
func (s *MemoryStore) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, m := range s.byID {
		if m.Status != StatusPublished {
			n++
		}
	}
	return n
}
