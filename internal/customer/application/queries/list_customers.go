package queries

import (
	"context"

	"github.com/example/myapp/internal/customer/application/dto"
	"github.com/example/myapp/pkg/pagination"
)

// ListCustomers returns a page of customers.
type ListCustomers struct {
	Page pagination.Page
}

// ListCustomersHandler executes ListCustomers.
type ListCustomersHandler struct{ deps Deps }

// NewListCustomersHandler wires the handler.
func NewListCustomersHandler(d Deps) *ListCustomersHandler {
	return &ListCustomersHandler{deps: d}
}

// Handle returns the requested page from the read model.
func (h *ListCustomersHandler) Handle(ctx context.Context, q ListCustomers) (pagination.Result[dto.Customer], error) {
	return h.deps.ReadModel.List(ctx, q.Page)
}
