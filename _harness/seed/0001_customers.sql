-- Fixture customers. Idempotent: re-running restores these rows to exactly
-- this state. Ids and emails use the harness- / @harness.test prefixes so
-- checks and stubs can refer to them, and so they can never collide with ids
-- the app generates.
--
-- Both sides are seeded: `customers` is the write model, `customer_reads` the
-- projection the query side (and order's customer check) reads from.

INSERT INTO customers (id, name, email, status, created_at, version) VALUES
    ('harness-cust-0001', 'Harness Seed One', 'seed-0001@harness.test', 'active', '2026-01-01T00:00:00Z', 1),
    ('harness-cust-0002', 'Harness Seed Two', 'seed-0002@harness.test', 'active', '2026-01-01T00:00:01Z', 1)
ON CONFLICT (id) DO UPDATE SET
    name       = EXCLUDED.name,
    email      = EXCLUDED.email,
    status     = EXCLUDED.status,
    created_at = EXCLUDED.created_at,
    version    = EXCLUDED.version;

INSERT INTO customer_reads (id, name, email, status, created_at) VALUES
    ('harness-cust-0001', 'Harness Seed One', 'seed-0001@harness.test', 'active', '2026-01-01T00:00:00Z'),
    ('harness-cust-0002', 'Harness Seed Two', 'seed-0002@harness.test', 'active', '2026-01-01T00:00:01Z')
ON CONFLICT (id) DO UPDATE SET
    name       = EXCLUDED.name,
    email      = EXCLUDED.email,
    status     = EXCLUDED.status,
    created_at = EXCLUDED.created_at;
