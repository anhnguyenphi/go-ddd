---
name: add-endpoint
description: Use when adding or changing an API endpoint (an RPC plus its REST route) in this repo. It walks the proto-first flow through the layers (proto → application command/query → gRPC adapter → module wiring) and starts with a failing regression scenario.
---

# Add an endpoint

REST is never hand-written. It's grpc-gateway, transcoded from the proto.
Copy the customer context: `CreateCustomer` for a command, `GetCustomer` /
`ListCustomers` for a query.

## Steps

1. **Tests first.** Add a scenario in `internal/testsupport/scenarios` (and
   list it in `All()`) for the new route: the happy path (status and body,
   then the read side via polling), a validation failure (`400`), and
   not-found (`404`) where relevant. It runs in e2e against your sandbox:
   run `make GO=/snap/go/current/bin/go harness-verify ARGS=--e2e` (or
   `harness-e2e ARGS='-run TestScenarios/<Name>'` once the sandbox is up) and
   see it fail. Optionally add scratch
   checks in `_harness/checks/<task>.sh` to watch rows (`--await`) and events
   (`--events`, `--no-events`) live.
2. **Proto**: add the RPC and messages to `api/proto/<ctx>/v1/<ctx>.proto`
   with a `google.api.http` annotation (`post: "/api/v1/..." body: "*"`).
   Field names are snake_case and become the REST JSON names. Then run
   `make proto && make proto-lint`. Commit the regenerated `*.pb.go`,
   `*.pb.gw.go`, and `api/openapi/myappv1.swagger.json`.
3. **Domain** (if needed): put invariants on the aggregate and value objects
   in `internal/<ctx>/domain`. Return `shareddomain.Invalid(...)`,
   `ErrNotFound`, or `ErrConflict`, never HTTP or gRPC errors.
4. **Application**: add a command in `application/commands/` (runs in
   `UoW.Within`, then `Repo.Save` + `Events.Publish(agg.PullEvents()...)`) or
   a query in `application/queries/` (reads through the `ReadModel` port
   only). Add focused unit tests with the generated mocks.
5. **gRPC adapter**: add a method on `interfaces/grpc/server.go`. It maps the
   request to the command or query and maps errors with `mapError`. For a
   create, set `grpcx.HTTPStatusMetadataKey` to `"201"` and
   `grpcx.LocationMetadataKey`, as `CreateCustomer` does. Without it, REST
   returns 200.
6. **Wiring**: construct the handler in the context's `module.go`.
   `RegisterGateway` already mounts every RPC of the service.
7. **New integration event?** See `add-consumer` for the consumer side. Add
   the producer mapping in `infrastructure/messaging/event_mappers.go`, pin
   the payload in `tests/contract`, and add a topic to `HARNESS_TOPICS` if it
   belongs to a new context.
8. Run `quality-gate`, then `verify-live`.

## Gotchas

- Status codes come from `mapError` (gRPC code), which grpc-gateway turns into
  HTTP: NotFound→404, AlreadyExists→409, InvalidArgument→400,
  PermissionDenied→403, anything else→500.
- The read side is eventually consistent. A scenario that reads right after
  a write polls (`api.Eventually`, `AwaitCustomer`); a scratch check needs a
  re-polled `GET` (the default for GET) or `--await`.
