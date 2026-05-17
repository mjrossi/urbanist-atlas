-- name: UpsertRegion :one
-- Idempotent insert/update of a region. Returns the row's ID.
INSERT INTO regions (country, kind, name, slug, scope_tier, sort_priority)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (slug) DO UPDATE
SET country = EXCLUDED.country,
    kind = EXCLUDED.kind,
    name = EXCLUDED.name,
    scope_tier = EXCLUDED.scope_tier,
    sort_priority = EXCLUDED.sort_priority
RETURNING id;

-- name: DeleteRegionParents :exec
-- Wholesale-replace pattern: clear a region's parent edges before
-- re-inserting them.
DELETE FROM region_parents WHERE region_id = $1;

-- name: InsertRegionParent :exec
INSERT INTO region_parents (region_id, parent_region_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RegionIDBySlug :one
SELECT id FROM regions WHERE slug = $1;

-- name: UpsertPostalCode :exec
INSERT INTO postal_codes (country, postal_code, leaf_region_id)
VALUES ($1, $2, $3)
ON CONFLICT (country, postal_code) DO UPDATE
SET leaf_region_id = EXCLUDED.leaf_region_id;
