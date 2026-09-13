-- customer bounded context: query-side read model. Denormalised and owned by
-- the projection in internal/customer/infrastructure/projections; never
-- written to directly by commands, and shaped for reads, not invariants (no
-- version column, no FKs). Kept eventually consistent from the `customers`
-- write table (migrations/0001) via integration events.
CREATE TABLE IF NOT EXISTS customer_reads (
    id         TEXT        PRIMARY KEY,
    name       TEXT        NOT NULL,
    email      TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- List() orders by (created_at, id) for stable pagination.
CREATE INDEX IF NOT EXISTS customer_reads_created_at_id_idx
    ON customer_reads (created_at, id);
