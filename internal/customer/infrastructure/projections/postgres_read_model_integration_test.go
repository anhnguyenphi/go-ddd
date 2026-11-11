//go:build integration

package projections_test

import (
	"os"
	"testing"

	"github.com/example/myapp/internal/customer/infrastructure/projections"
	"github.com/example/myapp/internal/customer/infrastructure/projections/projectiontest"
	"github.com/example/myapp/internal/testsupport/containers"
)

func TestMain(m *testing.M) { os.Exit(containers.RunMain(m)) }

func TestPostgresReadModel_Conformance(t *testing.T) {
	projectiontest.Run(t, func(t *testing.T) projections.ReadModel {
		return projections.NewPostgresReadModel(containers.PostgresDB(t))
	})
}
