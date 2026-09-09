package commands

import (
	"context"

	"github.com/example/myapp/internal/customer/domain"
)

// CreateCustomer registers a new customer.
type CreateCustomer struct {
	Name  string
	Email string
}

// CreateCustomerResult carries the identity of the new customer.
type CreateCustomerResult struct {
	CustomerID string
}

// CreateCustomerHandler executes CreateCustomer.
type CreateCustomerHandler struct{ deps Deps }

// NewCreateCustomerHandler wires the handler.
func NewCreateCustomerHandler(d Deps) *CreateCustomerHandler {
	return &CreateCustomerHandler{deps: d}
}

// Handle validates input, enforces the email-uniqueness policy, builds the
// aggregate, and persists it together with its events in one unit of work.
func (h *CreateCustomerHandler) Handle(ctx context.Context, cmd CreateCustomer) (CreateCustomerResult, error) {
	email, err := domain.NewEmail(cmd.Email)
	if err != nil {
		return CreateCustomerResult{}, err
	}
	id, err := domain.NewCustomerID(h.deps.IDs.NewID())
	if err != nil {
		return CreateCustomerResult{}, err
	}

	err = h.deps.UoW.Within(ctx, func(ctx context.Context) error {
		if err := h.deps.Uniqueness.EnsureAvailable(ctx, email); err != nil {
			return err
		}

		customer, err := domain.NewCustomer(id, cmd.Name, email, h.deps.Clock.Now())
		if err != nil {
			return err
		}
		if err := h.deps.Repo.Save(ctx, customer); err != nil {
			return err
		}
		return h.deps.Events.Publish(ctx, customer.PullEvents()...)
	})
	if err != nil {
		return CreateCustomerResult{}, err
	}
	return CreateCustomerResult{CustomerID: id.String()}, nil
}
