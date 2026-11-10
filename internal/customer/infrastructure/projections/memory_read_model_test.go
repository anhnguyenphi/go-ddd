package projections_test

import (
	"testing"

	"github.com/example/myapp/internal/customer/infrastructure/projections"
	"github.com/example/myapp/internal/customer/infrastructure/projections/projectiontest"
)

func TestMemoryReadModelConformance(t *testing.T) {
	projectiontest.Run(t, func(*testing.T) projections.ReadModel { return projections.NewMemoryReadModel() })
}
