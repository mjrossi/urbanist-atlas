-- Browse + recent queries — feed /api/v1/regions, /api/v1/regions/{slug},
-- and /api/v1/recent. See
-- docs/superpowers/specs/2026-05-18-browse-endpoints-design.md for the
-- original design and editorial decisions (descendant-walk org counts,
-- national-tier filter, 10-row cap). Endpoint went through two renames:
-- /metros -> /places (broadened to include cities) -> /regions
-- (broadened detail to any non-national slug; the list endpoint
-- ships without filter parameters today, but the SQL keeps the @kinds
-- parameter so a future filter slice can plumb a kinds arg through
-- without rewriting the query).
--
-- ListRegions walks the region DAG downward from each matched root
-- (parent->child relation) for org counts, and a second pass
-- upward for `browse_parent_slug`. DescendantRegions does the same
-- downward walk but returns full region rows (GetRegion combines it
-- with AncestorRegions to build the lookup-style scope set).
-- ListRecent doesn't walk — it filters orgs by whether ANY of their
-- region attachments is non-national.

-- name: ListRegions :many
-- Returns every region whose kind is in the caller-supplied $1 list
-- and has at least one approved organization attached to it directly
-- OR via a descendant in the region DAG, with the org count. The $1
-- kind set comes from the Postgres adapter — today it always passes
-- atlas.DefaultBrowseKindStrings() (metros + cities). The arg stays
-- parameterized so a future filter slice can override.
--
-- Always excludes scope_tier='national' regions (preserves the v1
-- editorial decision to keep national-tier content out of browse
-- contexts; same gate as /lookup). Always excludes regions with
-- zero approved orgs.
--
-- Each row also carries `browse_parent_slug`: the slug of the
-- nearest ancestor (walking upward via region_parents) whose kind
-- is also in $1. Lets clients group cities visually under their
-- parent metro without a second request. NULL for rows whose walk
-- doesn't reach a browseable-kind ancestor (typical for metros).
--
-- Three CTEs:
--   * `roots` — regions matching the kind filter.
--   * `descendants` — downward walk for the org-count.
--   * `ancestors` — upward walk for browse_parent_slug; the
--     `nearest_browseable_parent` aggregate picks the shallowest
--     hit per root and resolves ties alphabetically.
-- Ordered by org_count DESC then name ASC.
WITH RECURSIVE
roots AS (
    SELECT id, country, kind, name, slug, scope_tier, sort_priority
    FROM regions
    WHERE kind = ANY(@kinds::text[])
      AND scope_tier <> 'national'
),
descendants(root_id, region_id) AS (
    -- Seed: each root is its own descendant (an org tagged directly
    -- to the root must count).
    SELECT r.id, r.id FROM roots r
    UNION
    -- Recurse downward through region_parents. Filter scope_tier
    -- 'national' on the recursion so an editorial slip-up (a
    -- national region wired as a child of a metro) can't inflate the
    -- root's org_count via orgs attached only to that national row.
    -- Matches the sibling DescendantRegions CTE below.
    SELECT d.root_id, rp.region_id
    FROM descendants d
    JOIN region_parents rp ON rp.parent_region_id = d.region_id
    JOIN regions r2        ON r2.id = rp.region_id
    WHERE r2.scope_tier <> 'national'
),
ancestors(root_id, ancestor_id, depth) AS (
    -- Seed: direct parents of each root.
    SELECT r.id, rp.parent_region_id, 1
    FROM roots r
    JOIN region_parents rp ON rp.region_id = r.id
    UNION ALL
    -- Recurse upward through region_parents (child -> parent).
    -- Bounded by depth < 20 to defend against any unexpected
    -- cycle in the data (the DAG is acyclic by construction; the
    -- deepest legitimate walk in the v1 seed is ~5).
    SELECT a.root_id, rp.parent_region_id, a.depth + 1
    FROM ancestors a
    JOIN region_parents rp ON rp.region_id = a.ancestor_id
    WHERE a.depth < 20
),
nearest_browseable_parent AS (
    -- For each root, pick the shallowest ancestor whose kind is also
    -- in the @kinds set. Alphabetic tiebreak when multiple ancestors
    -- share min depth (e.g., a city with two browseable parents).
    SELECT DISTINCT ON (a.root_id) a.root_id, r.slug
    FROM ancestors a
    JOIN regions r ON r.id = a.ancestor_id
    WHERE r.kind = ANY(@kinds::text[])
      AND r.scope_tier <> 'national'
    ORDER BY a.root_id, a.depth ASC, r.slug ASC
)
SELECT
    r.id,
    r.country,
    r.kind,
    r.name,
    r.slug,
    r.scope_tier,
    r.sort_priority,
    nbp.slug AS browse_parent_slug,
    COUNT(DISTINCT o.id)::bigint AS org_count
FROM roots r
LEFT JOIN nearest_browseable_parent nbp ON nbp.root_id = r.id
JOIN descendants d            ON d.root_id = r.id
JOIN organization_regions orx ON orx.region_id = d.region_id
JOIN organizations o          ON o.id = orx.organization_id
WHERE o.status = 'approved'
GROUP BY r.id, r.country, r.kind, r.name, r.slug, r.scope_tier, r.sort_priority, nbp.slug
HAVING COUNT(DISTINCT o.id) > 0
ORDER BY org_count DESC, r.name ASC;

-- name: DescendantRegions :many
-- Returns the focus region followed by every descendant reachable by
-- walking region_parents in the parent->child direction. Ordered by
-- BFS depth (root first, then layer by layer). Excludes
-- scope_tier='national' rows from both the seed (defensive — the
-- focus is already gated by GetRegionBySlug) and the recursion.
--
-- Used by GetRegion to build the in-scope region set for the
-- downward direction of the lookup-style scope walk. The upward
-- direction is covered by AncestorRegions in lookup.sql.
WITH RECURSIVE descendants(id, country, kind, name, slug, scope_tier, sort_priority, depth) AS (
    SELECT r.id, r.country, r.kind, r.name, r.slug, r.scope_tier, r.sort_priority, 0
    FROM regions r
    WHERE r.id = $1 AND r.scope_tier <> 'national'
    UNION
    SELECT r.id, r.country, r.kind, r.name, r.slug, r.scope_tier, r.sort_priority, d.depth + 1
    FROM regions r
    JOIN region_parents rp ON rp.region_id = r.id
    JOIN descendants d     ON rp.parent_region_id = d.id
    WHERE r.scope_tier <> 'national'
),
deduped AS (
    SELECT DISTINCT ON (id) id, country, kind, name, slug, scope_tier, sort_priority, depth
    FROM descendants
    ORDER BY id, depth ASC
)
SELECT id, country, kind, name, slug, scope_tier, sort_priority
FROM deduped
ORDER BY depth ASC, sort_priority ASC, id ASC;

-- name: GetRegionBySlug :one
-- Returns the region identified by slug, with the only filter being
-- the national-tier exclusion. Resolves any non-national region —
-- metros, cities, counties, boroughs, states, multi-state coalitions.
-- Returns no row when the slug is unknown OR names a national-tier
-- region; the adapter maps the empty result to (nil, nil) → 404.
SELECT id, country, kind, name, slug, scope_tier, sort_priority
FROM regions
WHERE slug = @slug
  AND scope_tier <> 'national';

-- name: ListRecent :many
-- Returns the 10 most-recently-approved organizations, newest first.
-- Excludes orgs whose ONLY region attachments are scope_tier='national'
-- — the same default-lookup filter from slice #4.6, applied here so
-- a homepage "Recently added" strip can't accidentally surface MUBi
-- and friends. Orgs with at least one non-national attachment surface
-- normally.
--
-- The 10-row cap is hardcoded; opening it would require an OpenAPI
-- spec edit (see spec §1).
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
      SELECT 1
      FROM organization_regions orx
      JOIN regions r ON r.id = orx.region_id
      WHERE orx.organization_id = o.id
        AND r.scope_tier <> 'national'
  )
ORDER BY o.created_at DESC, o.id DESC
LIMIT 10;
