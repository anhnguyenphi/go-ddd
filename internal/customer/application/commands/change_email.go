package commands

import (
	"context"

	"github.com/example/myapp/internal/customer/domain"
	sharedapp "github.com/example/myapp/internal/shared/application"
)

// ChangeCustomerEmail changes an existing customer's email address.
type ChangeCustomerEmail struct {
	CustomerID string
	NewEmail   string
}

// ChangeCustomerEmailHandler executes ChangeCustomerEmail.
type ChangeCustomerEmailHandler struct{ deps Deps }

// NewChangeCustomerEmailHandler wires the handler.
func NewChangeCustomerEmailHandler(d Deps) *ChangeCustomerEmailHandler {
	return &ChangeCustomerEmailHandler{deps: d}
}

// Handle loads the aggregate, applies the change, enforces uniqueness of the new
// address, and persists the result plus its events atomically.
func (h *ChangeCustomerEmailHandler) Handle(ctx context.Context, cmd ChangeCustomerEmail) (sharedapp.None, error) {
	id, err := domain.NewCustomerID(cmd.CustomerID)
	if err != nil {
		return sharedapp.None{}, err
	}
	newEmail, err := domain.NewEmail(cmd.NewEmail)
	if err != nil {
		return sharedapp.None{}, err
	}

	err = h.deps.UoW.Within(ctx, func(ctx context.Context) error {
		customer, err := h.deps.Repo.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if customer.Email().Equals(newEmail) {
			return nil
		}
		if err := h.deps.Uniqueness.EnsureAvailable(ctx, newEmail); err != nil {
			return err
		}
		if err := customer.ChangeEmail(newEmail, h.deps.Clock.Now()); err != nil {
			return err
		}
		if err := h.deps.Repo.Save(ctx, customer); err != nil {
			return err
		}
		return h.deps.Events.Publish(ctx, customer.PullEvents()...)
	})
	return sharedapp.None{}, err
}
