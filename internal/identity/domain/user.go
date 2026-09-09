// Package domain is the identity context's domain layer (skeleton).
package domain

import (
	"time"

	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// UserID identifies a User aggregate.
type UserID struct{ value string }

// NewUserID validates a raw id.
func NewUserID(v string) (UserID, error) {
	if v == "" {
		return UserID{}, shareddomain.Invalid("user id must not be empty")
	}
	return UserID{value: v}, nil
}

// String returns the raw id.
func (id UserID) String() string { return id.value }

// User is the aggregate root (skeleton).
type User struct {
	shareddomain.AggregateRoot
	id           UserID
	email        string
	passwordHash string
	registeredAt time.Time
}

// UserRegistered is a domain event.
type UserRegistered struct {
	UserID string
	Email  string
	At     time.Time
}

func (e UserRegistered) EventName() string     { return "identity.user_registered" }
func (e UserRegistered) OccurredAt() time.Time { return e.At }
func (e UserRegistered) AggregateID() string   { return e.UserID }
