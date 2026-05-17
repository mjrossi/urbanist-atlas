-- Write queries against the regions table. Used by the `loadpostal`
-- subcommand to ingest postal-code crosswalks idempotently.
--
-- The slug column carries the UNIQUE constraint so it is the natural
-- conflict target; an UPDATE refreshes the rest of the row so a
-- corrected name/kind/scope_tier in the source file flows through to
-- the DB on the next run.

-- name: UpsertRegion :one
INSERT INTO regions (kind, name, slug, country, scope_tier)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (slug) DO UPDATE
    SET kind       = EXCLUDED.kind,
        name       = EXCLUDED.name,
        country    = EXCLUDED.country,
        scope_tier = EXCLUDED.scope_tier
RETURNING id;

-- name: GetRegionIDBySlug :one
SELECT id FROM regions WHERE slug = $1;
