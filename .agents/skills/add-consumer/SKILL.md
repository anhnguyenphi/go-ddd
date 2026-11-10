---
name: add-consumer
description: Use when a bounded context must react to an integration event (a new subscriber, projection, or cross-context reaction), or when adding a new integration event. It covers contract-only coupling, idempotency, wiring, topics, tests, and live checks in the sandbox.
---

# Add an event consumer

Examples to copy: `internal/notification` (a cross-context consumer),
`internal/customer/infrastructure/projections` (a read-model projection), and
`internal/customer/infrastructure/messaging/event_handlers.go` (an inbound
reaction).

## Steps

1. **Tests first.** In `tests/integration`, publish the event the way its
   producer would and assert the side effect (see
   `TestKafka_ExternallyProducedEventOnPrefixedTopics` in
   `side_effects_test.go`; `testsupport.WithLogCapture` for log-only effects).
   If the event comes from an HTTP command in this repo, also assert the
   effect end to end on every profile (`TestSideEffects`, or a scenario when a
   client can see it). To watch it live in your sandbox, add a scratch check
   in `_harness/checks/<task>.sh`:
   ```bash
   id="$(new_id)"
   check --name worker-<event>-<effect> --produce <ctx>.v1.<event> --key "$id" \
     --body "{\"id\":\"$id\", ...}" \
     --await "SELECT ... WHERE id = '$id'" '<value>'     # or --logs / --calls / --events
   ```
   `--produce` also fails the check if the message hits `.retry.N` or `.dlq`.
2. **Contract, not code**: the consumer depends on the event name string and
   its payload shape only. Declare a local payload struct that mirrors the
   producer's (see `notification/application/handlers`). Never import the
   producer context (`tests/arch` enforces this).
3. **Handler**: `func(ctx, eventbus.Event) error`. It must be **idempotent**
   (delivery is at least once), for example an upsert or `ON CONFLICT`.
   Return an error only when a retry could help. An error sends the message
   to `.retry.1`, then `.dlq`.
4. **Wire it**: subscribe in the context's `module.go`
   (`RegisterSubscriptions`), which `bootstrap/modules.go` calls. With Kafka,
   a subscription must exist before `Run`. All wiring happens in the
   bootstrap, so this is automatic.
5. **Topic**: the topic is `<ctx>.v1.events` (`kafka.DefaultTopicFor`; with
   `KAFKA_TOPIC_PREFIX`, `<prefix>.<ctx>.v1.events`). If it's a new context,
   add it to `HARNESS_TOPICS` in `_harness/.env.harness`.
6. **Failure path**: unit-test that the handler returns an error for an
   undecodable payload. Retry → DLQ delivery itself is pinned by
   `tests/integration/kafka_test.go`. To see it live, a scratch check with an
   undecodable payload, `--dead-letter`, and
   `--expect-errors '"msg":"event handler (failed|exhausted retries, sent to dead-letter topic)"'`
   (see `_harness/checks/example.sh.sample`).
7. Run `quality-gate`, then `verify-live`.

## New event (producer side)

Add the name constant and payload struct in the producer's
`infrastructure/messaging/event_mappers.go`, map it in `ToIntegrationEvent`,
pin it in `tests/contract`, and while developing, watch it with
`--events <name>@<id>` in a scratch check. A payload change that isn't additive needs a new `v2` name.
