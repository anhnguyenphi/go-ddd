# Opt-in suite: failure paths of the Kafka consumer side.
#   make harness-verify ARGS="--suite resilience"
#   make harness-smoke  ARGS="--suite resilience"
#
# Not in the default run because it makes handlers fail on purpose.

# A payload the projection can't decode must hop through the retry topic and
# land on the DLQ, without blocking the partition for the next message.
poison="$(new_id)"
check --name worker-poison-message-dead-lettered \
	--produce customer.v1.created --key "$poison" \
	--body "{\"id\":42,\"name\":\"Harness Poison\",\"email\":\"poison-$poison@harness.test\"}" \
	--dead-letter --timeout 20 \
	--logs "event handler exhausted retries, sent to dead-letter topic" \
	--expect-errors "\"msg\":\"event handler (failed|exhausted retries, sent to dead-letter topic)\""

# The partition keeps flowing after the poison message.
after="$(new_id)"
check --name worker-recovers-after-poison-message \
	--produce customer.v1.created --key "$after" \
	--body "{\"id\":\"$after\",\"name\":\"Harness After Poison\",\"email\":\"after-$after@harness.test\"}" \
	--await "SELECT count(*) FROM customer_reads WHERE id = '$after'" 1
