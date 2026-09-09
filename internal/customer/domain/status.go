package domain

import shareddomain "github.com/example/myapp/internal/shared/domain"

// Status is the lifecycle state of a Customer.
type Status string

const (
	StatusActive      Status = "active"
	StatusDeactivated Status = "deactivated"
)

// ParseStatus validates a raw status string.
func ParseStatus(v string) (Status, error) {
	switch Status(v) {
	case StatusActive:
		return StatusActive, nil
	case StatusDeactivated:
		return StatusDeactivated, nil
	default:
		return "", shareddomain.Invalid("unknown customer status: " + v)
	}
}

// String returns the raw status.
func (s Status) String() string { return string(s) }
