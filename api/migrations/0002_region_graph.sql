-- 0002_region_graph.sql
--
-- Replaces the US/CA-shaped 4-slot postal_codes denormalization with a
-- region DAG. See docs/superpowers/specs/2026-05-16-region-graph-design.md
-- for the full design rationale.
--
-- DESTRUCTIVE: this migration drops all data in regions,
-- organization_regions, and postal_codes. The seed data is reloaded
-- via `just loaddata` after the migration runs. Safe pre-Phase-1
-- (no real data) and pre-Phase-2 (only dogfood data); not safe after
-- Phase 2 — that's when proper backfill migrations start mattering.

-- +goose Up

-- Drop tables that depend on regions, then regions itself, in FK order.
DROP TABLE IF EXISTS organization_regions;
DROP TABLE IF EXISTS postal_codes;
DROP TABLE IF EXISTS regions;

-- regions: free-form kind (no CHECK), explicit scope_tier, sort_priority.
-- +goose StatementBegin
CREATE TABLE regions (
    id            BIGSERIAL PRIMARY KEY,
    country       TEXT NOT NULL,
    kind          TEXT NOT NULL,
    name          TEXT NOT NULL,
    slug          TEXT NOT NULL UNIQUE,
    scope_tier    TEXT NOT NULL CHECK (scope_tier IN ('local','regional')),
    sort_priority INT  NOT NULL DEFAULT 50
);
-- +goose StatementEnd

CREATE INDEX regions_country_idx    ON regions (country);
CREATE INDEX regions_scope_tier_idx ON regions (scope_tier);
CREATE INDEX regions_kind_idx       ON regions (kind);

-- region_parents: the DAG. Multi-parent allowed; CHECK blocks self-loops;
-- longer cycles are caught at write-time in loadregions.
-- +goose StatementBegin
CREATE TABLE region_parents (
    region_id        BIGINT NOT NULL REFERENCES regions(id) ON DELETE CASCADE,
    parent_region_id BIGINT NOT NULL REFERENCES regions(id) ON DELETE CASCADE,
    PRIMARY KEY (region_id, parent_region_id),
    CHECK (region_id <> parent_region_id)
);
-- +goose StatementEnd

CREATE INDEX region_parents_parent_idx ON region_parents (parent_region_id);

-- postal_codes: single pointer to the leaf region. Ancestor walk
-- happens at lookup time via recursive CTE.
-- +goose StatementBegin
CREATE TABLE postal_codes (
    postal_code    TEXT   NOT NULL,
    country        TEXT   NOT NULL,
    leaf_region_id BIGINT NOT NULL REFERENCES regions(id) ON DELETE RESTRICT,
    PRIMARY KEY (country, postal_code)
);
-- +goose StatementEnd

CREATE INDEX postal_codes_leaf_idx ON postal_codes (leaf_region_id);

-- organization_regions: unchanged in shape; recreated because we just
-- dropped regions and CASCADE wiped the join table.
-- +goose StatementBegin
CREATE TABLE organization_regions (
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    region_id       BIGINT NOT NULL REFERENCES regions(id)       ON DELETE CASCADE,
    PRIMARY KEY (organization_id, region_id)
);
-- +goose StatementEnd

CREATE INDEX organization_regions_region_idx ON organization_regions (region_id);

-- +goose Down

-- Restore the 0001 schema shape. Note: data is NOT restored on rollback;
-- after a down migration you'd need to re-run loadpostal + seed with the
-- old format, which no longer exists in this repo. In practice 0002 is
-- forward-only.
DROP TABLE IF EXISTS organization_regions;
DROP TABLE IF EXISTS postal_codes;
DROP TABLE IF EXISTS region_parents;
DROP TABLE IF EXISTS regions;

CREATE TABLE regions (
    id          BIGSERIAL PRIMARY KEY,
    kind        TEXT NOT NULL CHECK (kind IN ('city','county','metro','state','province','country','multi-state')),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    country     TEXT NOT NULL CHECK (country IN ('US','CA')),
    parent_id   BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    scope_tier  TEXT NOT NULL CHECK (scope_tier IN ('local','regional'))
);
CREATE TABLE postal_codes (
    postal_code      TEXT   NOT NULL,
    country          TEXT   NOT NULL CHECK (country IN ('US','CA')),
    city_region_id   BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    county_region_id BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    metro_region_id  BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    state_region_id  BIGINT REFERENCES regions(id) ON DELETE SET NULL,
    PRIMARY KEY (country, postal_code)
);
CREATE TABLE organization_regions (
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    region_id       BIGINT NOT NULL REFERENCES regions(id)       ON DELETE CASCADE,
    PRIMARY KEY (organization_id, region_id)
);
