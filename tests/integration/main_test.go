//go:build integration

// Package integration wires the whole application (bootstrap.Build) in-process
// and checks it end to end:
//
//   - profiles_test.go runs the shared black-box scenarios
//     (internal/testsupport/scenarios) against every persistence/bus profile:
//     memory, postgres, postgres+kafka.
//   - adapters_test.go runs the adapter conformance suites (repotest,
//     projectiontest) and the SQL unit of work against real Postgres.
//   - migrations_test.go checks every migration rolls back and re-applies.
//   - kafka_test.go checks the Kafka bus against a real broker, including
//     retry hops and the dead-letter topic.
//
// Real infrastructure comes from testcontainers (internal/testsupport/
// containers). Without Docker those tests skip (fail with REQUIRE_INFRA=1);
// with -short they always skip, leaving the memory profile.
//
//	go test -tags=integration ./tests/integration/...          # all
//	go test -tags=integration -short ./tests/integration/...   # memory only
package integration

import (
	"os"
	"testing"

	"github.com/example/myapp/internal/testsupport/containers"
)

func TestMain(m *testing.M) { os.Exit(containers.RunMain(m)) }
