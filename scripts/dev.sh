#!/usr/bin/env bash
# Run the API with debug logging and a fast outbox loop for local development.
set -euo pipefail
cd "$(dirname "$0")/.."

export LOG_LEVEL="${LOG_LEVEL:-debug}"
export LOG_FORMAT="${LOG_FORMAT:-text}"
export OUTBOX_POLL_INTERVAL="${OUTBOX_POLL_INTERVAL:-200ms}"

GO="${GO:-go}"
exec "$GO" run ./cmd/api
