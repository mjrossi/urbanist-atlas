-- 0004_split_nyc.sql
--
-- NYC borough split (slice #7.5.2). The existing single `nyc` region
-- becomes a regional intermediate region between the 5 borough leaves
-- and `nyc-metro`. State edges (the `nyc → ny` parent edge) migrate
-- from `nyc` to each borough, per region-graph rule §1 ("state edges
-- live on the leaf, not on the metro" —
-- docs/region-graph.md).
--
-- Data-only migration; no schema changes. Idempotent under goose's
-- per-version run guarantee:
--   - fresh DB (no `nyc` row yet) — every UPDATE/DELETE/INSERT matches
--     0 rows and the migration is a no-op.
--   - DB pre-seeded with the old shape (nyc.scope_tier='local',
--     parents=[nyc-metro, ny]) — the migration transforms it.
--
-- Subsequent `loaddata` runs upsert the new regions_us.toml shape on
-- top, which idempotently re-applies the same edges via the upsert
-- + wholesale-replace pattern in internal/loadregions.
--
-- Verified pre-migration: api/seed/postal_codes_us.csv has no rows
-- anchoring at `nyc` directly (all NYC ZIPs already anchor at borough
-- slugs: brooklyn, manhattan, queens, bronx, staten-island). The
-- migration therefore leaves postal_codes untouched.
--
-- See docs/superpowers/specs/2026-05-19-postal-coverage-design.md for
-- the design rationale.

-- +goose Up

-- nyc flips from local leaf to regional intermediate.
UPDATE regions
   SET scope_tier = 'regional'
 WHERE slug = 'nyc'
   AND scope_tier = 'local';

-- Drop the (nyc, ny) parent edge: post-split, boroughs carry the
-- state edge instead. Subquery returns 0 rows on a fresh DB; the
-- DELETE then matches 0 rows.
DELETE FROM region_parents
 WHERE region_id        = (SELECT id FROM regions WHERE slug = 'nyc')
   AND parent_region_id = (SELECT id FROM regions WHERE slug = 'ny');

-- Add (borough, ny) parent edges for each of the 5 boroughs that
-- exist in the DB. ON CONFLICT DO NOTHING keeps re-runs harmless; on
-- a fresh DB the SELECT side is empty so no rows are inserted.
INSERT INTO region_parents (region_id, parent_region_id)
SELECT b.id, ny.id
  FROM regions b
  CROSS JOIN regions ny
 WHERE ny.slug = 'ny'
   AND b.slug IN ('brooklyn', 'manhattan', 'queens', 'bronx', 'staten-island')
ON CONFLICT DO NOTHING;

-- +goose Down

-- Drop the borough → ny edges that the Up step added (or that an
-- equivalent loaddata run would have added).
DELETE FROM region_parents
 WHERE parent_region_id = (SELECT id FROM regions WHERE slug = 'ny')
   AND region_id IN (
        SELECT id FROM regions
         WHERE slug IN ('brooklyn', 'manhattan', 'queens', 'bronx', 'staten-island')
   );

-- Restore the (nyc, ny) parent edge.
INSERT INTO region_parents (region_id, parent_region_id)
SELECT nyc.id, ny.id
  FROM regions nyc, regions ny
 WHERE nyc.slug = 'nyc' AND ny.slug = 'ny'
ON CONFLICT DO NOTHING;

UPDATE regions
   SET scope_tier = 'local'
 WHERE slug = 'nyc'
   AND scope_tier = 'regional';
