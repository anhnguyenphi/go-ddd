package domain

// ValueObject is a marker for immutable, attribute-defined concepts (Email,
// Money, Address, ...). Value objects are compared by value and must be
// self-validating at construction time.
//
// Equals lets generic code compare value objects without reflection; concrete
// types normally also support Go's == when all fields are comparable.
type ValueObject[T any] interface {
	Equals(other T) bool
}
