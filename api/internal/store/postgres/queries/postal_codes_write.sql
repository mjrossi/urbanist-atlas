-- Write queries against postal_codes. Used by the `loadpostal`
-- subcommand to ingest postal-code crosswalks idempotently.
--
-- (country, postal_code) is the PK so it's the natural conflict
-- target. An UPDATE refreshes the region FKs so the table tracks the
-- latest crosswalk.

-- name: UpsertPostalCode :exec
INSERT INTO postal_codes (
    postal_code, country, city_region_id, county_region_id, metro_region_id, state_region_id
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (country, postal_code) DO UPDATE
    SET city_region_id   = EXCLUDED.city_region_id,
        county_region_id = EXCLUDED.county_region_id,
        metro_region_id  = EXCLUDED.metro_region_id,
        state_region_id  = EXCLUDED.state_region_id;
