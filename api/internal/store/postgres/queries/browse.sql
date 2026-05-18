-- Browse + recent queries — feed /api/v1/metros, /api/v1/metros/{slug},
-- and /api/v1/recent. See
-- docs/superpowers/specs/2026-05-18-browse-endpoints-design.md for the
-- design and editorial decisions (metro-kind set, national-tier filter,
-- 10-row cap).
--
-- All three queries walk the region DAG. ListMetros and OrgsForMetro
-- descend from the metro region (child-of relation) so an org tagged
-- only to Brooklyn counts toward NYC metro. ListRecent doesn't walk —
-- it filters orgs by whether ANY of their region attachments is
-- non-national.

-- name: ListMetros :many
-- Returns every metro-equivalent region (kind = ANY($1::text[])) that
-- has at least one approved organization attached to it directly OR via
-- a descendant in the region DAG, with the org count.
--
-- Excludes scope_tier='national' regions defensively even though no
-- known metro-kind currently has that tier. Excludes metros with zero
-- approved orgs (those would be a confusing UI element on the Browse
-- panel — a metro with no advocacy to browse).
--
-- The descendants CTE walks region_parents in the parent->child
-- direction (parent_region_id = current id) starting from each metro.
-- We materialize the (metro_id, descendant_id) pairs and aggregate.
-- Ordered by org_count DESC then name ASC (alphabetical tiebreak
-- keeps the response stable when counts collide).
WITH RECURSIVE
metros AS (
    SELECT id, country, kind, name, slug, scope_tier, sort_priority
    FROM regions
    WHERE kind = ANY(@kinds::text[])
      AND scope_tier <> 'national'
),
descendants(metro_id, region_id) AS (
    -- Seed: each metro is its own descendant (an org tagged directly
    -- to the metro must count).
    SELECT m.id, m.id FROM metros m
    UNION
    -- Recurse downward through region_parents.
    SELECT d.metro_id, rp.region_id
    FROM descendants d
    JOIN region_parents rp ON rp.parent_region_id = d.region_id
)
SELECT
    m.id,
    m.country,
    m.kind,
    m.name,
    m.slug,
    m.scope_tier,
    m.sort_priority,
    COUNT(DISTINCT o.id)::bigint AS org_count
FROM metros m
JOIN descendants d         ON d.metro_id = m.id
JOIN organization_regions orx ON orx.region_id = d.region_id
JOIN organizations o       ON o.id = orx.organization_id
WHERE o.status = 'approved'
GROUP BY m.id, m.country, m.kind, m.name, m.slug, m.scope_tier, m.sort_priority
HAVING COUNT(DISTINCT o.id) > 0
ORDER BY org_count DESC, m.name ASC;

-- name: GetMetroBySlug :one
-- Returns the metro region identified by slug AND kind ∈ metro kinds.
-- Returns no row when the slug is unknown OR names a non-metro region;
-- the adapter maps the empty result to (nil, nil) → 404.
SELECT id, country, kind, name, slug, scope_tier, sort_priority
FROM regions
WHERE slug = @slug
  AND kind = ANY(@kinds::text[])
  AND scope_tier <> 'national';

-- name: OrgsForMetro :many
-- For a given metro region id, returns every approved org with at
-- least one attachment in the metro's downward DAG closure (the metro
-- itself + every descendant region). For each org we also array_agg
-- ALL its region attachments so the adapter can hydrate Org.Regions
-- in one round-trip.
--
-- The recursion prunes scope_tier='national' nodes defensively. Today
-- the metro is always non-national (GetMetroBySlug enforces it) and
-- editorial policy keeps national tiers as graph roots, so the prune
-- is a no-op on the current seed. It guards against a future edge
-- that accidentally puts a national region under a metro, which would
-- otherwise leak national-only orgs into a metro detail page.
--
-- Ordered by o.created_at DESC, then o.id DESC for stability.
WITH RECURSIVE descendants(region_id) AS (
    SELECT @metro_id::bigint
    UNION
    SELECT rp.region_id
    FROM descendants d
    JOIN region_parents rp ON rp.parent_region_id = d.region_id
    JOIN regions r2        ON r2.id = rp.region_id
    WHERE r2.scope_tier <> 'national'
)
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
      WHERE oj.organization_id = o.id
        AND oj.region_id IN (SELECT region_id FROM descendants)
  )
ORDER BY o.created_at DESC, o.id DESC;

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
