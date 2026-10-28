# Default smoke checks: run by every `make harness-verify`.
#
# One `check` per behavior; the DSL is documented in
# _harness/scripts/check_dsl.sh. Add the check for a behavior you're about to
# change BEFORE changing the code, and watch it fail first.
#
# Fixtures from _harness/seed: harness-cust-0001, harness-cust-0002.
# Use $(new_id) for anything a check creates, so re-runs never collide.

# ---- ops ----------------------------------------------------------------------

check --name api-healthz --path /healthz --status 200 --jq '.status == "ok"'

check --name api-readyz-db-reachable --path /readyz --status 200 --jq '.status == "ready"'

check --name api-openapi-served --path /openapi.json --status 200 \
	--jq '.paths | has("/api/v1/customers") and has("/api/v1/orders")'

# ---- customer: write path -> outbox -> Kafka -> projection + notification ------

email="create-$(new_id)@harness.test"
check --name customer-create-happy --method POST --path /api/v1/customers \
	--body "{\"name\":\"Harness Create\",\"email\":\"$email\"}" \
	--status 201 --jq '.id | length > 0' --save customer_id=.id \
	--events customer.v1.created \
	--await "SELECT email FROM customers WHERE email = '$email'" "$email" \
	--await "SELECT status FROM customer_reads WHERE email = '$email'" active \
	--logs "\"msg\":\"welcome email sent\".*\"to\":\"$email\""
customer_id="$(var customer_id)"

check --name customer-get-from-read-model --path "/api/v1/customers/$customer_id" \
	--status 200 --jq ".id == \"$customer_id\" and .email == \"$email\" and .status == \"active\""

check --name customer-create-duplicate-email --method POST --path /api/v1/customers \
	--body "{\"name\":\"Harness Dup\",\"email\":\"$email\"}" \
	--status 409 --no-events

check --name customer-create-invalid-email --method POST --path /api/v1/customers \
	--body '{"name":"Harness Invalid","email":"not-an-email"}' \
	--status 400 --no-events

check --name customer-create-missing-fields --method POST --path /api/v1/customers \
	--body '{}' --status 400 --no-events

check --name customer-get-unknown --path /api/v1/customers/harness-does-not-exist --status 404

check --name customer-list-includes-seed --path '/api/v1/customers?limit=100' --status 200 \
	--jq '[.items[].id] | index("harness-cust-0001") != null and index("harness-cust-0002") != null' \
	--jq '.total >= 2'

new_email="changed-$(new_id)@harness.test"
check --name customer-change-email --method PATCH --path "/api/v1/customers/$customer_id/email" \
	--body "{\"email\":\"$new_email\"}" --status 200 \
	--events "customer.v1.email_changed@$customer_id" \
	--await "SELECT email FROM customers WHERE id = '$customer_id'" "$new_email" \
	--await "SELECT email FROM customer_reads WHERE id = '$customer_id'" "$new_email"

check --name customer-change-email-unknown --method PATCH \
	--path /api/v1/customers/harness-does-not-exist/email \
	--body '{"email":"nobody@harness.test"}' --status 404 --no-events

# ---- order: synchronous cross-context call into customer ----------------------

check --name order-place-for-seeded-customer --method POST --path /api/v1/orders \
	--body '{"customer_id":"harness-cust-0001"}' --status 200 --jq '.id | length > 0'

check --name order-place-unknown-customer --method POST --path /api/v1/orders \
	--body '{"customer_id":"harness-does-not-exist"}' --status 404 --no-events

# ---- async: events produced straight onto Kafka, as another producer would -----

id="$(new_id)"
produced_email="produced-$id@harness.test"
check --name worker-customer-created-projects-and-notifies \
	--produce customer.v1.created --key "$id" \
	--body "{\"id\":\"$id\",\"name\":\"Harness Produced\",\"email\":\"$produced_email\"}" \
	--await "SELECT email FROM customer_reads WHERE id = '$id'" "$produced_email" \
	--logs "\"msg\":\"welcome email sent\".*\"to\":\"$produced_email\""

order_id="$(new_id)"
check --name worker-order-placed-observed-by-customer \
	--produce order.v1.placed --key "$order_id" \
	--body "{\"id\":\"$order_id\",\"customer_id\":\"harness-cust-0001\"}" \
	--logs "customer context observed order.v1.placed.*\"aggregate_id\":\"$order_id\""
