-- 0001_init.sql
--
-- Initial schema for Urbanist Atlas. Five tables that together describe
-- the geographic-resolution + organization-directory model laid out in
-- the design doc:
--
--   - regions             — a geographic unit an org can serve
--                           (city / county / metro / state / province /
--                            country / multi-state). scope_tier drives
--                            "Local" vs "Regional" bucketing in /lookup.
--   - postal_codes        — US ZIPs and Canadian FSAs (3-char) joined
--                           to their containing regions.
--   - organizations       — the directory entries themselves.
--   - organization_regions — many-to-many join: an org may serve
--                            multiple regions, and a region may host
--                            multiple orgs.
--   - submissions         — the public submission queue. Approval
--                            atomically promotes a submission into an
--                            organization row.
--
-- Slice #5 (submissions handlers) lands later; the table is here in
-- 0001 so the schema is a single coherent unit and so seed loaders can
-- reason about all five tables without staging.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE regions (
    id          BIGSERIAL PRIMARY KEY,
    kind        TEXT NOT NULL CHECK (kind IN (
                    'city','county','metro','state','province','country','multi-state'
                )),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    country     TEXT NOT NULL CHECK (country IN ('US','CA')),
    parent_id   BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    scope_tier  TEXT NOT NULL CHECK (scope_tier IN ('local','regional'))
);
-- +goose StatementEnd

CREATE INDEX regions_kind_idx        ON regions (kind);
CREATE INDEX regions_country_idx     ON regions (country);
CREATE INDEX regions_scope_tier_idx  ON regions (scope_tier);

-- +goose StatementBegin
CREATE TABLE postal_codes (
    postal_code      TEXT   NOT NULL,
    country          TEXT   NOT NULL CHECK (country IN ('US','CA')),
    city_region_id   BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    county_region_id BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    metro_region_id  BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    state_region_id  BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    PRIMARY KEY (country, postal_code)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE organizations (
    id           BIGSERIAL PRIMARY KEY,
    slug         TEXT        NOT NULL UNIQUE,
    name         TEXT        NOT NULL,
    short_desc   TEXT        NOT NULL,
    website_url  TEXT        NOT NULL,
    contact_url  TEXT,
    tags         TEXT[]      NOT NULL DEFAULT '{}'::text[],
    status       TEXT        NOT NULL CHECK (status IN ('approved','pending','rejected','archived')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_at  TIMESTAMPTZ
);
-- +goose StatementEnd

CREATE INDEX organizations_status_idx      ON organizations (status);
CREATE INDEX organizations_approved_at_idx ON organizations (approved_at DESC NULLS LAST);

-- +goose StatementBegin
CREATE TABLE organization_regions (
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    region_id       BIGINT NOT NULL REFERENCES regions(id)       ON DELETE CASCADE,
    PRIMARY KEY (organization_id, region_id)
);
-- +goose StatementEnd

CREATE INDEX organization_regions_region_idx ON organization_regions (region_id);

-- +goose StatementBegin
CREATE TABLE submissions (
    id               BIGSERIAL PRIMARY KEY,
    payload          JSONB       NOT NULL,
    submitter_name   TEXT,
    submitter_email  TEXT,
    submitter_note   TEXT,
    status           TEXT        NOT NULL CHECK (status IN ('pending','approved','rejected')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at     TIMESTAMPTZ,
    promoted_org_id  BIGINT REFERENCES organizations(id) ON DELETE SET NULL,
    rejection_reason TEXT
);
-- +goose StatementEnd

CREATE INDEX submissions_status_idx     ON submissions (status);
CREATE INDEX submissions_created_at_idx ON submissions (created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS submissions;
DROP TABLE IF EXISTS organization_regions;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS postal_codes;
DROP TABLE IF EXISTS regions;
