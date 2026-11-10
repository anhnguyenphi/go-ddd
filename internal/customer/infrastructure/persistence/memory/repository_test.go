package memory_test

import (
	"testing"

	"github.com/example/myapp/internal/customer/domain"
	"github.com/example/myapp/internal/customer/infrastructure/persistence/memory"
	"github.com/example/myapp/internal/customer/infrastructure/persistence/repotest"
)

func TestRepositoryConformance(t *testing.T) {
	repotest.Run(t, func(*testing.T) domain.Repository { return memory.New() })
}
