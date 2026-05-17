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
