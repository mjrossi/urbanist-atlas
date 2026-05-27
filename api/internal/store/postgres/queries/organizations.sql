-- name: UpsertOrganization :one
INSERT INTO organizations (slug, name, short_desc, website_url, contact_url, tags, status, approved_at)
VALUES ($1, $2, $3, $4, $5, $6, 'approved', NOW())
ON CONFLICT (slug) DO UPDATE
SET name = EXCLUDED.name,
    short_desc = EXCLUDED.short_desc,
    website_url = EXCLUDED.website_url,
    contact_url = EXCLUDED.contact_url,
    tags = EXCLUDED.tags
RETURNING id;

-- name: DeleteOrganizationRegions :exec
DELETE FROM organization_regions WHERE organization_id = $1;

-- name: InsertOrganizationRegion :exec
INSERT INTO organization_regions (organization_id, region_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RegionIDsBySlugs :many
SELECT id, slug FROM regions WHERE slug = ANY($1::text[]);

-- name: GetOrgBySlug :one
-- Returns the approved organization identified by slug, with every
-- region it serves array_agg'd onto the row so the adapter can hydrate
-- Org.Regions in one round-trip. Mirrors the shape used by
-- ListRecent and OrgsForRegionsAndAllRegionIDs so the Postgres-side
-- adapter (hydrateOrgRows) can
-- be reused without a new mapper.
--
-- Returns no row when the slug is unknown OR names an org whose status
-- is not 'approved' (e.g. pending or rejected submissions); the adapter
-- maps the empty result to ErrOrgNotFound → 404.
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
WHERE o.slug = @slug
  AND o.status = 'approved';
