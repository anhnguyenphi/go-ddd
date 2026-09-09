-- Transactional outbox. Rows are inserted in the SAME transaction as the
-- aggregate change; the relay (cmd/worker) forwards them to the bus.
CREATE TABLE IF NOT EXISTS outbox_messages (
    id             TEXT        PRIMARY KEY,        -- Event.ID (dedupe key)
    name           TEXT        NOT NULL,           -- e.g. customer.v1.created
    source         TEXT        NOT NULL,
    aggregate_id   TEXT        NOT NULL,
    version        INTEGER     NOT NULL DEFAULT 1,
    correlation_id TEXT        NOT NULL DEFAULT '',
    causation_id   TEXT        NOT NULL DEFAULT '',
    payload        JSONB       NOT NULL,
    metadata       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    status         TEXT        NOT NULL DEFAULT 'pending',  -- pending|published|failed
    attempts       INTEGER     NOT NULL DEFAULT 0,
    last_error     TEXT        NOT NULL DEFAULT '',
    occurred_at    TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ
);

-- The relay polls this: "oldest pending first".
CREATE INDEX IF NOT EXISTS outbox_messages_pending_idx
    ON outbox_messages (created_at)
    WHERE status <> 'published';
