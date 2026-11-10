// Package containers starts throwaway infrastructure for infra integration
// tests via testcontainers-go. Each container is started lazily, once per test
// binary, and shared by its tests; isolation comes from a fresh database per
// test (PostgresDSN) and a unique consumer group per test (UniqueGroupID).
//
// When Docker is unavailable the tests skip, unless REQUIRE_INFRA=1 (set in
// CI), in which case they fail. With -short they always skip.
//
// Call RunMain from TestMain so containers are terminated when the binary
// exits (Ryuk reaps them anyway if the process is killed).
package containers

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

// Images match _harness/docker-compose.yml so both test layers see the same
// server versions.
const (
	PostgresImage = "postgres:16-alpine"
	RedpandaImage = "redpandadata/redpanda:v24.2.7"
)

var (
	mu         sync.Mutex
	terminates []func(context.Context) error
)

func onTerminate(c testcontainers.Container) {
	mu.Lock()
	defer mu.Unlock()
	terminates = append(terminates, func(ctx context.Context) error { return c.Terminate(ctx) })
}

// RunMain runs the tests and then terminates every container they started.
//
//	func TestMain(m *testing.M) { os.Exit(containers.RunMain(m)) }
func RunMain(m *testing.M) int {
	code := m.Run()
	mu.Lock()
	defer mu.Unlock()
	for _, term := range terminates {
		_ = term(context.Background())
	}
	return code
}

// Require skips the test in -short mode. Call it first in infra tests.
func Require(t testing.TB) {
	t.Helper()
	if testing.Short() {
		t.Skip("infra test: skipped with -short")
	}
}

// unavailable skips (or, with REQUIRE_INFRA=1, fails) when a container could
// not be started.
func unavailable(t testing.TB, what string, err error) {
	t.Helper()
	if os.Getenv("REQUIRE_INFRA") == "1" {
		t.Fatalf("%s unavailable and REQUIRE_INFRA=1: %v", what, err)
	}
	t.Skipf("%s unavailable (is Docker running?): %v", what, err)
}

var (
	rootOnce sync.Once
	root     string
)

// RepoRoot returns the module root: the nearest ancestor of the working
// directory (the test's package dir under `go test`) that holds go.mod.
func RepoRoot() string {
	rootOnce.Do(func() {
		dir, _ := os.Getwd()
		for dir != "/" && dir != "." {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				root = dir
				return
			}
			dir = filepath.Dir(dir)
		}
	})
	return root
}
