// Package services holds customer domain services — domain logic that does not
// naturally belong to a single aggregate instance.
package services

import (
	"context"

	"github.com/example/myapp/internal/customer/domain"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// EmailUniqueness enforces the rule "an email address identifies at most one
// customer". It is a domain service because the check spans the whole
// collection, which a single aggregate cannot see.
type EmailUniqueness struct {
	repo domain.Repository
}

// NewEmailUniqueness wires the policy to the repository.
func NewEmailUniqueness(repo domain.Repository) *EmailUniqueness {
	return &EmailUniqueness{repo: repo}
}

// EnsureAvailable returns shared/domain.ErrConflict if email is already taken.
func (p *EmailUniqueness) EnsureAvailable(ctx context.Context, email domain.Email) error {
	taken, err := p.repo.ExistsByEmail(ctx, email)
	if err != nil {
		return err
	}
	if taken {
		return shareddomain.ErrConflict
	}
	return nil
}
