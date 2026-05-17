-- Write queries for organizations + organization_regions. Used by the
-- `seed` subcommand to upsert hand-curated YAML data idempotently.
--
-- Seeded orgs are always inserted with status='approved' and an
-- approved_at timestamp; the submission queue (slice #5) is the only
-- way new pending rows enter the table.
--
-- The slug column is the conflict target on organizations; the
-- (organization_id, region_id) PK is the conflict target on
-- organization_regions. ReplaceOrganizationRegions is the
-- straightforward "delete + reinsert inside the same transaction"
-- pattern — the alternative (diff in app code) is more code for the
-- same on-disk result.

-- name: UpsertOrganization :one
INSERT INTO organizations (
    slug, name, short_desc, website_url, contact_url, tags, status, approved_at
)
VALUES ($1, $2, $3, $4, $5, $6, 'approved', NOW())
ON CONFLICT (slug) DO UPDATE
    SET name        = EXCLUDED.name,
        short_desc  = EXCLUDED.short_desc,
        website_url = EXCLUDED.website_url,
        contact_url = EXCLUDED.contact_url,
        tags        = EXCLUDED.tags,
        status      = 'approved',
        approved_at = COALESCE(organizations.approved_at, NOW())
RETURNING id;

-- name: DeleteOrganizationRegions :exec
DELETE FROM organization_regions WHERE organization_id = $1;

-- name: InsertOrganizationRegion :exec
INSERT INTO organization_regions (organization_id, region_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;
