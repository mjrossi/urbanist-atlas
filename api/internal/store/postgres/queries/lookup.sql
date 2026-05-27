-- name: ResolveLeafRegion :one
-- Returns the leaf region for a normalized postal code.
SELECT r.id, r.country, r.kind, r.name, r.slug, r.scope_tier, r.sort_priority
FROM postal_codes pc
JOIN regions r ON r.id = pc.leaf_region_id
WHERE pc.country = $1 AND pc.postal_code = $2;

-- name: AncestorRegions :many
-- Returns the leaf followed by all transitive ancestors, ordered
-- most-specific first (BFS layer order). UNION (not UNION ALL) in the
-- recursion gives Postgres the termination signal, but UNION dedupes on
-- the full tuple including `depth` — so a region reachable at multiple
-- depths via a DAG diamond surfaces as multiple rows. The outer
-- DISTINCT ON (id) collapses those, keeping the smallest depth (i.e.
-- the most-specific traversal). Example: ZIP 20017's leaf is
-- `washington-dc`, whose parents are `[washington-dc-metro, dc]`; the
-- metro's parents include `dc` again, so `dc` is reachable at depth 1
-- and depth 2. We want it once, at depth 1.
--
-- Excludes scope_tier='national' regions from both branches: national
-- regions are filtered out of the default lookup surface (see
-- docs/superpowers/specs/2026-05-17-region-graph-pt-validation-design.md
-- §2). This is defense-in-depth — the data shape intentionally avoids
-- parent edges from the leaf chain into national regions, but the
-- filter ensures national-tier orgs stay hidden even if an edge is
-- added in error.
WITH RECURSIVE ancestors(id, country, kind, name, slug, scope_tier, sort_priority, depth) AS (
    SELECT r.id, r.country, r.kind, r.name, r.slug, r.scope_tier, r.sort_priority, 0
    FROM regions r
    WHERE r.id = $1 AND r.scope_tier <> 'national'
    UNION
    -- Bounded by depth < 20 as a belt-and-suspenders against any
    -- unexpected cycle in the data. Matches the descendants CTEs in
    -- browse.sql.
    SELECT r.id, r.country, r.kind, r.name, r.slug, r.scope_tier, r.sort_priority, a.depth + 1
    FROM regions r
    JOIN region_parents rp ON rp.parent_region_id = r.id
    JOIN ancestors a       ON rp.region_id = a.id
    WHERE r.scope_tier <> 'national'
      AND a.depth < 20
),
deduped AS (
    SELECT DISTINCT ON (id) id, country, kind, name, slug, scope_tier, sort_priority, depth
    FROM ancestors
    ORDER BY id, depth ASC
)
SELECT id, country, kind, name, slug, scope_tier, sort_priority
FROM deduped
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
--
-- Includes o.created_at so the adapter can populate Org.CreatedAt —
-- atlas.Store's contract is that OrgsForRegions returns the same
-- shape ListRecent does (the storetest harness pins this).
SELECT
    o.id, o.slug, o.name, o.short_desc, o.website_url, o.contact_url, o.tags,
    o.created_at,
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
