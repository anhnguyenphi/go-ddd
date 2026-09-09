package domain

import "errors"

// Sentinel error kinds shared across contexts. Contexts wrap these with %w so
// that delivery adapters can map them to transport status codes centrally
// (see each context's interfaces/http/response.go).
var (
	// ErrNotFound — the requested aggregate does not exist.
	ErrNotFound = errors.New("domain: not found")
	// ErrConflict — the operation violates an invariant that depends on other
	// aggregates or on uniqueness (e.g. email already taken).
	ErrConflict = errors.New("domain: conflict")
	// ErrValidation — the input is not acceptable to the domain.
	ErrValidation = errors.New("domain: validation failed")
	// ErrPermission — the caller is not allowed to perform the operation.
	ErrPermission = errors.New("domain: permission denied")
)

// ValidationError is a convenience for attaching a message to ErrValidation
// while keeping errors.Is(err, ErrValidation) true.
type ValidationError struct{ Msg string }

func (e ValidationError) Error() string { return "domain: " + e.Msg }
func (e ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// Invalid builds a ValidationError.
func Invalid(msg string) error { return ValidationError{Msg: msg} }
