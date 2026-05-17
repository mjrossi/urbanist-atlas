-- name: ResolveLeafRegion :one
-- Returns the leaf region for a normalized postal code.
SELECT r.id, r.country, r.kind, r.name, r.slug, r.scope_tier, r.sort_priority
FROM postal_codes pc
JOIN regions r ON r.id = pc.leaf_region_id
WHERE pc.country = $1 AND pc.postal_code = $2;

-- name: AncestorRegions :many
-- Returns the leaf followed by all transitive ancestors, ordered
-- most-specific first (BFS layer order). UNION (not UNION ALL)
-- deduplicates DAG diamonds and gives Postgres the termination signal.
WITH RECURSIVE ancestors(id, country, kind, name, slug, scope_tier, sort_priority, depth) AS (
    SELECT r.id, r.country, r.kind, r.name, r.slug, r.scope_tier, r.sort_priority, 0
    FROM regions r WHERE r.id = $1
    UNION
    SELECT r.id, r.country, r.kind, r.name, r.slug, r.scope_tier, r.sort_priority, a.depth + 1
    FROM regions r
    JOIN region_parents rp ON rp.parent_region_id = r.id
    JOIN ancestors a       ON rp.region_id = a.id
)
SELECT id, country, kind, name, slug, scope_tier, sort_priority
FROM ancestors
ORDER BY depth ASC, sort_priority ASC, id ASC;

-- name: ParentSlugsForRegions :many
-- Returns (region_id, parent_slug) rows so the adapter can populate
-- Region.ParentSlugs without a per-region round-trip.
SELECT rp.region_id, r.slug AS parent_slug
FROM region_parents rp
JOIN regions r ON r.id = rp.parent_region_id
WHERE rp.region_id = ANY($1::bigint[])
ORDER BY rp.region_id, r.slug;

-- name: OrgsForRegionsAndAllRegionIDs :many
-- For each org with at least one attachment in the queried set,
-- returns the org row plus ALL the region IDs that org is attached
-- to (array_agg). The adapter then hydrates each ID into a Region via
-- GetRegionsByIDs in one round-trip.
SELECT
    o.id, o.slug, o.name, o.short_desc, o.website_url, o.contact_url, o.tags,
    ARRAY(
        SELECT orx.region_id
        FROM organization_regions orx
        WHERE orx.organization_id = o.id
        ORDER BY orx.region_id
    )::bigint[] AS region_ids
FROM organizations o
WHERE o.status = 'approved'
  AND EXISTS (
      SELECT 1 FROM organization_regions oj
      WHERE oj.organization_id = o.id AND oj.region_id = ANY($1::bigint[])
  )
ORDER BY o.id;

-- name: GetRegionsByIDs :many
-- Hydrates a set of region IDs into rows (no parent_slugs here; those
-- come from ParentSlugsForRegions when needed).
SELECT id, country, kind, name, slug, scope_tier, sort_priority
FROM regions
WHERE id = ANY($1::bigint[]);
