-- Queries against postal_codes + regions for the lookup pipeline.
--
-- The Postgres adapter normalizes user input in Go (uppercase, strip
-- whitespace, truncate CA codes to FSA) before calling these — same
-- rules MemStore applies. See pkg/atlas/memstore.go for the source of
-- truth on normalization.

-- name: ResolvePostalCode :one
-- Look up the four region IDs (city, county, metro, state/province)
-- for a normalized (country, postal_code) pair.
SELECT
    postal_code,
    country,
    city_region_id,
    county_region_id,
    metro_region_id,
    state_region_id
FROM postal_codes
WHERE country     = $1
  AND postal_code = $2;

-- name: GetRegionsByIDs :many
-- Fetch full region rows for a set of region IDs. Used both to
-- materialize the ResolvedPostalCode (city/county/metro/state) and to
-- populate Org.Regions on lookup results.
SELECT
    id,
    kind,
    name,
    slug,
    country,
    scope_tier
FROM regions
WHERE id = ANY($1::bigint[]);
