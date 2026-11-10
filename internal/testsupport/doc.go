// Package testsupport holds helpers shared by the integration and e2e suites.
// It is test-only: production packages must never import it (enforced by
// tests/arch). Subpackages:
//
//   - testsupport (this package): NewApp wires the full application via
//     bootstrap.Build for in-process integration tests, in any profile
//     (in-memory, Postgres, Kafka).
//   - testsupport/containers: throwaway Postgres and Redpanda via
//     testcontainers-go, one container per test binary, one fresh database
//     per test.
//   - testsupport/api: a black-box REST client, Eventually, and unique-data
//     fixtures. It never imports the application, so tests/e2e can use it
//     against any deployment.
package testsupport
