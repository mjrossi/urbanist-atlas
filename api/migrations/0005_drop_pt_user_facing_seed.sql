-- 0005_drop_pt_user_facing_seed.sql
--
-- Remove the Portugal validation-fixture data from production.
--
-- Background: PT was loaded by `loaddata.LoadAll` starting in slice
-- #4.6 as a region-graph stress test (multi-parent municípios, AML's
-- cross-NUTS-II span, autonomous-region parallel hierarchy, uniões
-- de freguesias). The model held up; the validation served its
-- purpose. The slice #25 hygiene pass scopes v1 user-facing
-- functionality to US + CA only, so PT rows on the QA / prod DB
-- are now stale data leaking into /metros, /recent, and /lookup.
--
-- The PT region and postal-code seed files stay under api/seed/
-- as a reference and for the integration suite (pipeline_test.go
-- loads them explicitly via loadregions.LoadFile +
-- loadpostal.LoadFile). The four PT orgs were removed from
-- orgs.toml in the same slice so a re-run of loaddata won't
-- reintroduce them.
--
-- Data-only migration; no schema changes. Deletions are in FK-safe
-- order and idempotent — a fresh DB with no PT rows is a no-op.

-- +goose Up

-- Strip the join-table edges before the rows on either side go away.
-- +goose StatementBegin
DELETE FROM organization_regions
 WHERE region_id IN (SELECT id FROM regions WHERE country = 'PT');
-- +goose StatementEnd

-- Drop the four PT orgs by slug. Targeting by slug (not by joined
-- region) catches the mubi-nacional row whose only attachment is
-- the national-tier pt-nacional region — orgs that attach only to
-- PT regions can't survive the regions delete below either way, but
-- being explicit here makes the intent unambiguous on re-read.
-- +goose StatementBegin
DELETE FROM organizations
 WHERE slug IN ('mubi-lisboa', 'mubi-porto', 'lisboa-para-pessoas', 'mubi-nacional');
-- +goose StatementEnd

-- PT postal codes (the 7 entries from the validation fixture).
-- +goose StatementBegin
DELETE FROM postal_codes WHERE country = 'PT';
-- +goose StatementEnd

-- Region DAG edges first, then the regions themselves. region_parents
-- has FK to regions on both sides, so both directions need clearing.
-- +goose StatementBegin
DELETE FROM region_parents
 WHERE region_id IN (SELECT id FROM regions WHERE country = 'PT')
    OR parent_region_id IN (SELECT id FROM regions WHERE country = 'PT');
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM regions WHERE country = 'PT';
-- +goose StatementEnd


-- +goose Down
--
-- Intentional no-op. PT is no longer part of loaddata.LoadAll and
-- the orgs were removed from orgs.toml in the same slice, so the
-- "down" direction can't be a simple re-seed. If a developer wants
-- PT data back in the dev DB:
--
--   just loadregions seed/regions_pt.toml PT
--   just loadpostal  seed/postal_codes_pt.csv PT
--
-- The orgs would need a separate PT-orgs TOML or a one-off insert.

-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd
