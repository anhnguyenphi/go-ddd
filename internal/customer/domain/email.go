package domain

import (
	"strings"

	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// Email is a validated, normalised email address value object.
type Email struct{ value string }

// NewEmail validates and normalises (trim + lowercase) an address. The check is
// deliberately conservative — a single "@" with non-empty local and domain
// parts and a dot in the domain. Delivery is what really validates an address.
func NewEmail(raw string) (Email, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return Email{}, shareddomain.Invalid("email must not be empty")
	}
	at := strings.IndexByte(v, '@')
	if at <= 0 || at == len(v)-1 {
		return Email{}, shareddomain.Invalid("email is not a valid address")
	}
	if strings.Count(v, "@") != 1 {
		return Email{}, shareddomain.Invalid("email is not a valid address")
	}
	if !strings.Contains(v[at+1:], ".") {
		return Email{}, shareddomain.Invalid("email domain is not valid")
	}
	return Email{value: v}, nil
}

// String returns the normalised address.
func (e Email) String() string { return e.value }

// Equals implements a value-object comparison.
func (e Email) Equals(other Email) bool { return e.value == other.value }
