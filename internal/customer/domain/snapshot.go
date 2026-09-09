package domain

import "time"

// Snapshot is a flat, exported projection of a Customer's state. It is the only
// sanctioned way for infrastructure (repositories) to read and reconstruct an
// aggregate without breaking encapsulation. It is NOT a wire/API type.
type Snapshot struct {
	ID        string
	Name      string
	Email     string
	Status    string
	CreatedAt time.Time
	Version   int
}

// ToSnapshot captures the aggregate's current state.
func (c *Customer) ToSnapshot() Snapshot {
	return Snapshot{
		ID:        c.id.String(),
		Name:      c.name,
		Email:     c.email.String(),
		Status:    c.status.String(),
		CreatedAt: c.createdAt,
		Version:   c.version,
	}
}

// FromSnapshot rebuilds an aggregate from persisted state. No events are
// recorded and all invariants are assumed to have held when the snapshot was
// written; it still re-validates the value objects so corrupt rows fail loudly.
func FromSnapshot(s Snapshot) (*Customer, error) {
	id, err := NewCustomerID(s.ID)
	if err != nil {
		return nil, err
	}
	email, err := NewEmail(s.Email)
	if err != nil {
		return nil, err
	}
	status, err := ParseStatus(s.Status)
	if err != nil {
		return nil, err
	}
	return &Customer{
		id:        id,
		name:      s.Name,
		email:     email,
		status:    status,
		createdAt: s.CreatedAt,
		version:   s.Version,
	}, nil
}
