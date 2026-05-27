# Migration conventions

This file documents conventions every new migration under
`api/migrations/` should follow. The embedded migration FS is loaded
by `cmd/server migrate` and applied at deploy time via Fly's
`release_command`, so the runtime cost of a bad migration is "next
deploy fails" at best and "production data lost" at worst — be
deliberate.

## Destructive migrations require an opt-in guard

Any migration that drops a column / table or truncates rows MUST gate
the destructive step behind a `current_setting('atlas.allow_destructive', …)`
check. The default is **refuse** — operators set the GUC explicitly
via `SET atlas.allow_destructive = 'on';` for the run that intends to
drop data. The runbook for the destructive deploy should call this
out so the SET happens via a one-shot `psql` session rather than
becoming a sticky default.

Pattern (mirrors the safety check 0003's `Down` already uses for the
`national` rollback):

```sql
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM <target_table> LIMIT 1)
       AND current_setting('atlas.allow_destructive', true) IS DISTINCT FROM 'on'
    THEN
        RAISE EXCEPTION
            'Migration %% would drop existing % data. Set '
            'atlas.allow_destructive=on for this session if intended.',
            '<migration_id>', '<target>';
    END IF;
END $$;
-- +goose StatementEnd
```

`current_setting(name, true)` is the missing-OK form — returns NULL
when the GUC isn't set rather than erroring. `IS DISTINCT FROM`
handles NULL as "not 'on'" without a separate `IS NULL` branch.

Rationale: pre-launch (current state) the DB only carried dogfood
data, and re-runs were idempotent because the worst case was
re-loading seed files. Post-launch the same migration shape silently
destroys user data unless we trip an explicit alarm — the alarm
forces a deliberate operator action so the destructive deploy can't
be the result of a stray `migrate up` from a recovery shell.

## Pre-launch migrations are grandfathered

Migrations 0001 – 0005 (the v1 schema bring-up + slice #4.6 national
tier + slice #7.5.2 NYC borough split + slice #25 PT seed drop)
shipped under the pre-launch policy where data loss had no real
blast radius. They do **not** carry the guard above, and they do
**not** need to be retrofitted. The first destructive migration
authored under the new convention is where the pattern starts.

## Idempotence

Every migration that loads or modifies data should be safe to re-run.
The two patterns the loaders rely on are:

  - `INSERT … ON CONFLICT (key) DO UPDATE SET …` for upserts.
  - `INSERT … ON CONFLICT DO NOTHING` for join-table edges.

A migration that needs to apply a one-off data transformation (e.g.
0004's NYC borough split) should be a no-op on a fresh DB — written
so the `UPDATE`/`DELETE`/`INSERT` SELECTs match zero rows when the
target shape already holds.

## Down direction

Forward-only migrations are acceptable when the down direction can't
reasonably restore the prior data shape (0002 and 0005 both fall here).
In that case, the `Down` section should contain a no-op statement
(`SELECT 1;`) wrapped in `-- +goose StatementBegin / StatementEnd`
so goose doesn't fail on an empty Down — and a comment block above
explaining why a real rollback isn't offered. See 0005's Down section
for the canonical shape.

## Statement framing

Use `-- +goose StatementBegin` / `-- +goose StatementEnd` blocks
around any multi-line statement (CREATE TABLE, DO $$ … $$, multi-line
INSERT). Single-line statements can be left bare. Goose splits on
semicolons by default and gets confused by semicolons inside
`DO $$ … $$` blocks without the explicit framing.
