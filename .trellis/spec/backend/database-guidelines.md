# Database Guidelines

> Database patterns and conventions for this project.

---

## Overview

- Driver: `pgx/v5` (via `jackc/pgx/v5/pgconn` for `PgError`).
- Migrations: numbered SQL files under `src/backend/migrations/`, applied
  by the server on startup (see `cmd/server/migrations_test.go` for the
  idempotency contract).
- Repo layer: hand-written SQL through `*pgxpool.Pool` or `sqlx`-style
  scans. No GORM.

---

## Query Patterns

- For optional UUID columns populated from string parameters, cast after
  `NULLIF`: `NULLIF($n, '')::uuid`. Without the explicit cast, PostgreSQL
  can infer the expression as `text` and fail inserts with SQLSTATE
  `42804`.
- Repo methods must propagate errors with `fmt.Errorf("... : %w", err)`
  (note `%w`, not `%v`) so that `errors.As(err, &pgErr)` works at the
  service layer.

---

## Migrations

### File Naming

`<NNN>_<short_underscore_name>.sql`, where `NNN` is the next sequential
number (zero-padded to 3 digits). Latest known: `047_unique_orchestrator_per_conversation.sql`.

### Structure

Each migration file has UP and DOWN sections separated by `---- DOWN`:

```sql
-- description of what this migration does and why
CREATE TABLE ...;

---- DOWN
DROP TABLE ...;
```

The runner executes only the part before `---- DOWN` on startup. The
DOWN section is documentation / manual rollback.

### Idempotency

UP section must be idempotent — `CREATE INDEX IF NOT EXISTS`, `CREATE
TABLE IF NOT EXISTS`, etc. The server runs migrations on every boot and
must not fail on the second run. See
`cmd/server/migrations_test.go:TestMigrationsAreIdempotentForRepeatedStartup`.

### Backward-Compatible Data Changes

When adding a constraint that may reject existing rows, **clean the data
before adding the constraint** in the same migration. Example from
`047_unique_orchestrator_per_conversation.sql`:

```sql
-- Demote surplus orchestrators so the unique index can be created.
UPDATE conversation_agents ca
SET role = 'worker'
WHERE role = 'orchestrator'
  AND joined_at > (
      SELECT MIN(joined_at)
      FROM conversation_agents ca2
      WHERE ca2.conversation_id = ca.conversation_id
        AND ca2.role = 'orchestrator'
  );

CREATE UNIQUE INDEX IF NOT EXISTS uq_conversation_agents_single_orchestrator
    ON conversation_agents (conversation_id)
    WHERE role = 'orchestrator';
```

Without the `UPDATE`, the index creation would fail on any conversation
that had two historical orchestrators (pre-constraint data).

---

## Naming Conventions

- **Tables**: `snake_case`, plural (`conversations`, `conversation_agents`).
- **Columns**: `snake_case`, singular (`conversation_id`, `joined_at`).
- **Indexes**: `idx_<table>_<cols>` (non-unique), `uq_<table>_<purpose>`
  (unique).
- **Foreign keys**: implicit via `REFERENCES`; do not name them unless
  the project already does so.
- **Partial unique indexes**: include the predicate in the name when
  useful (`uq_conversation_agents_single_orchestrator`), since the
  purpose is not obvious from the columns alone.

---

## Partial Unique Index Pattern

Use a partial unique index when a uniqueness constraint applies only to
a subset of rows.

```sql
CREATE UNIQUE INDEX IF NOT EXISTS <index_name>
    ON <table> (<cols>)
    WHERE <predicate>;
```

Examples:

- One orchestrator per conversation: `WHERE role = 'orchestrator'`.
- One active record per user: `WHERE deleted_at IS NULL`.

Application code that performs the write must detect the unique
violation and translate it into a domain conflict error (see
[Error Handling](./error-handling.md)). Detection helper:
`service.isUniqueViolation(err)`.

---

## Common Mistakes

- `NULLIF($n, '')` is not enough for UUID columns. Use
  `NULLIF($n, '')::uuid`, especially for optional reply anchors such as
  `source_message_id` and `dispatch_message_id`.
- Creating a UNIQUE INDEX without first cleaning existing duplicates
  fails the migration on production data. Always pair the index with an
  `UPDATE` (or `DELETE`) that resolves the conflict.
- Returning errors with `%v` instead of `%w` from repo methods breaks
  `errors.As` chains; the service layer cannot detect specific
  SQLSTATE codes.
- Skipping `IF NOT EXISTS` on indexes breaks idempotent startup.
