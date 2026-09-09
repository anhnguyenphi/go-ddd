package queries

import (
	"context"

	"github.com/example/myapp/internal/customer/application/dto"
)

// GetCustomer looks up one customer by id.
type GetCustomer struct {
	CustomerID string
}

// GetCustomerHandler executes GetCustomer.
type GetCustomerHandler struct{ deps Deps }

// NewGetCustomerHandler wires the handler.
func NewGetCustomerHandler(d Deps) *GetCustomerHandler {
	return &GetCustomerHandler{deps: d}
}

// Handle returns the read model or shared/domain.ErrNotFound.
func (h *GetCustomerHandler) Handle(ctx context.Context, q GetCustomer) (dto.Customer, error) {
	return h.deps.ReadModel.GetByID(ctx, q.CustomerID)
}
