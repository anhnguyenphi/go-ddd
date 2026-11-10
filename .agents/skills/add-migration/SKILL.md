---
name: add-migration
description: Use when changing the database schema (a new table, column, or index, or a data backfill). It covers writing the up/down pair, then applying, rolling back, and re-applying on the sandbox DB, and updating the seed and tests.
---

# Add a migration

## Steps

1. **Name**: use the next number, as `migrations/NNNN_<what>.up.sql` and
   `NNNN_<what>.down.sql` (look at `ls migrations` for the last number).
   Each file runs in one transaction using the simple protocol, so several
   statements per file are fine.
2. **Write both directions.** `down` must undo `up` exactly. Prefer
   `IF NOT EXISTS` / `IF EXISTS`. Make the change safe for a running app:
   add columns as nullable or with a default, and backfill separately before
   adding a constraint.
3. **Prove it on your sandbox's DB** (the sandbox must be up: `make harness-up`):
   ```bash
   make GO=/snap/go/current/bin/go harness-migrate ARGS=up
   make GO=/snap/go/current/bin/go harness-migrate ARGS='down 1'
   make GO=/snap/go/current/bin/go harness-migrate ARGS=up
   make harness-migrate ARGS=status
   ```
   Inspect the result:
   `docker compose -p myapp-harness -f _harness/docker-compose.yml exec -T postgres psql -U myapp -d myapp_sb_<sandbox> -c '\d <table>'`.
4. **Code**: update the Postgres repository or projection
   (`internal/<ctx>/infrastructure/...`). Keep the in-memory adapter's
   behavior identical. It's what the unit and integration tests use.
5. **Seed**: if the new column or table matters for fixtures, update
   `_harness/seed/*.sql` and keep it idempotent (`ON CONFLICT ... DO UPDATE`).
6. **Tests**: the adapter conformance suites (`repotest`, `projectiontest`)
   and `TestMigrations_DownAllThenUpAgain` in `make test-integration` pin it.
   While developing, a scratch check can watch the new state live:
   `--await "SELECT <col> FROM ..." '<value>'`.
7. Run `verify-live`. The report's `Schema:` line shows the new version.

## Gotchas

- `cmd/migrate` takes flags before the direction (`-steps 1 down`).
  `harness-migrate ARGS='down 1'` handles that for you.
- The `outbox_messages` table exists, but nothing writes to it yet (the
  outbox is in memory).
