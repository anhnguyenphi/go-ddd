-- customer bounded context: aggregate table.
CREATE TABLE IF NOT EXISTS customers (
    id         TEXT        PRIMARY KEY,
    name       TEXT        NOT NULL,
    email      TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    version    INTEGER     NOT NULL DEFAULT 1
);

-- email uniqueness is a domain invariant (see domain/services/email_uniqueness.go);
-- enforce it at the storage layer too.
CREATE UNIQUE INDEX IF NOT EXISTS customers_email_key ON customers (email);
