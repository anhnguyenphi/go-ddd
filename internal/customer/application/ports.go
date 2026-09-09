// Package application declares the customer context's driven ports that are not
// already covered by shared/application or the domain. Command/query handlers
// live in the commands/ and queries/ sub-packages.
package application

import (
	"context"

	"github.com/example/myapp/internal/customer/application/dto"
	"github.com/example/myapp/pkg/pagination"
)

// ReadModel is the query side (CQRS). It is fed by a projection that subscribes
// to customer integration events, so it is eventually consistent with the
// write side. Implemented in infrastructure/projections.
type ReadModel interface {
	GetByID(ctx context.Context, id string) (dto.Customer, error)
	List(ctx context.Context, page pagination.Page) (pagination.Result[dto.Customer], error)
}
