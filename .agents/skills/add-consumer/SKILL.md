---
name: add-consumer
description: Use when a bounded context must react to an integration event (a new subscriber, projection, or cross-context reaction), or when adding a new integration event. It covers contract-only coupling, idempotency, wiring, topics, and async harness checks.
---

# Add an event consumer

Examples to copy: `internal/notification` (a cross-context consumer),
`internal/customer/infrastructure/projections` (a read-model projection), and
`internal/customer/infrastructure/messaging/event_handlers.go` (an inbound
reaction).

## Steps

1. **Checks first**, in `_harness/smoke/checks.sh`. Produce the event the way
   its producer would, and assert the side effect:
   ```bash
   id="$(new_id)"
   check --name worker-<event>-<effect> --produce <ctx>.v1.<event> --key "$id" \
     --body "{\"id\":\"$id\", ...}" \
     --await "SELECT ... WHERE id = '$id'" '<value>'     # or --logs / --calls / --events
   ```
   `--produce` also fails the check if the message hits `.retry.N` or `.dlq`.
   If the event comes from an HTTP command in this repo, add an end-to-end
   variant that triggers the command and asserts the same effect.
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
5. **Topic**: the topic is `<ctx>.v1.events` (`kafka.DefaultTopicFor`). If
   it's a new context, add it to `HARNESS_TOPICS` in `_harness/.env.harness`.
6. **Failure path**: add a check to `_harness/smoke/checks_resilience.sh` with
   an undecodable payload, `--dead-letter`, and
   `--expect-errors '"msg":"event handler (failed|exhausted retries, sent to dead-letter topic)"'`.
7. Unit-test the handler. Run `quality-gate`, then `verify-live` (with
   `ARGS='--suite resilience'` too).

## New event (producer side)

Add the name constant and payload struct in the producer's
`infrastructure/messaging/event_mappers.go`, map it in `ToIntegrationEvent`,
pin it in `tests/contract`, and assert it with `--events <name>@<id>` in the
command's check. A payload change that isn't additive needs a new `v2` name.
