package bootstrap

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/example/myapp/internal/customer/domain"
	custmemory "github.com/example/myapp/internal/customer/infrastructure/persistence/memory"
	custpostgres "github.com/example/myapp/internal/customer/infrastructure/persistence/postgres"
	"github.com/example/myapp/internal/customer/infrastructure/projections"
	"github.com/example/myapp/internal/platform/config"
	"github.com/example/myapp/internal/platform/database"
	sharedapp "github.com/example/myapp/internal/shared/application"
	"github.com/example/myapp/internal/shared/infrastructure/transaction"
)

// persistence bundles the storage-layer choices for all contexts so they share
// one DB / one UnitOfWork (and therefore one transaction per request).
type persistence struct {
	db                *sql.DB // nil in the in-memory profile
	unitOfWork        sharedapp.UnitOfWork
	customerRepo      domain.Repository
	customerReadModel projections.ReadModel
}

// newPersistence picks the in-memory or Postgres profile based on config.
func newPersistence(ctx context.Context, cfg config.Config, logger *slog.Logger) (persistence, error) {
	if !cfg.UseDatabase() {
		logger.Info("persistence: in-memory")
		return persistence{
			unitOfWork:        transaction.Noop{},
			customerRepo:      custmemory.New(),
			customerReadModel: projections.NewMemoryReadModel(),
		}, nil
	}

	logger.Info("persistence: postgres")
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return persistence{}, err
	}
	return persistence{
		db:                db,
		unitOfWork:        transaction.SQL{DB: db},
		customerRepo:      custpostgres.New(db),
		customerReadModel: projections.NewPostgresReadModel(db),
	}, nil
}

func (p persistence) close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}
