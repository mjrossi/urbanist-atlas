-- Queries against organizations + organization_regions for the lookup
-- pipeline.
--
-- Only approved orgs are ever returned; rejected/pending/archived
-- statuses are filtered server-side rather than relying on calling code
-- to remember.

-- name: OrgsForRegionsAndAllRegionIDs :many
-- Return the distinct approved organizations that serve any of the
-- supplied region IDs, together with the full list of region IDs each
-- org serves (not just the ones that matched). The adapter joins those
-- region IDs against the regions table to materialize Org.Regions.
--
-- Returning region_ids as a column lets us answer the lookup with two
-- round-trips total (this + one GetRegionsByIDs), regardless of how
-- many orgs match.
SELECT
    o.id,
    o.slug,
    o.name,
    o.short_desc,
    o.website_url,
    o.contact_url,
    o.tags,
    ARRAY(
        SELECT orr.region_id
        FROM organization_regions orr
        WHERE orr.organization_id = o.id
        ORDER BY orr.region_id
    )::bigint[] AS region_ids
FROM organizations o
WHERE o.status = 'approved'
  AND EXISTS (
      SELECT 1
      FROM organization_regions orr
      WHERE orr.organization_id = o.id
        AND orr.region_id = ANY($1::bigint[])
  )
ORDER BY o.id;
