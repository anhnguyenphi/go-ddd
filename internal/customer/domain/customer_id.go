package domain

import shareddomain "github.com/example/myapp/internal/shared/domain"

// CustomerID is the identity of a Customer aggregate. It is a value object:
// immutable and validated at construction.
type CustomerID struct{ value string }

// NewCustomerID wraps a pre-existing id string (e.g. from persistence or an id
// generator). An empty id is rejected.
func NewCustomerID(v string) (CustomerID, error) {
	if v == "" {
		return CustomerID{}, shareddomain.Invalid("customer id must not be empty")
	}
	return CustomerID{value: v}, nil
}

// MustCustomerID is NewCustomerID for trusted inputs (tests, generated ids).
func MustCustomerID(v string) CustomerID {
	cid, err := NewCustomerID(v)
	if err != nil {
		panic(err)
	}
	return cid
}

// String returns the raw identifier.
func (id CustomerID) String() string { return id.value }

// IsZero reports whether the id is unset.
func (id CustomerID) IsZero() bool { return id.value == "" }

// Equals implements a value-object comparison.
func (id CustomerID) Equals(other CustomerID) bool { return id.value == other.value }
