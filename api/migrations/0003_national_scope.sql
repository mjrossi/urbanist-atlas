-- 0003_national_scope.sql
--
-- Expand regions.scope_tier to include 'national' as a third bucket.
-- See docs/superpowers/specs/2026-05-17-region-graph-pt-validation-design.md
-- for the editorial policy that distinguishes when 'national' applies
-- vs when an org should be modeled as 'regional' (with state chapter).
--
-- Default lookup behavior continues to bucket only 'local' + 'regional'.
-- The 'national' tier exists in the schema to allow national-scope orgs
-- (MUBi national for PT, Living Streets for UK, Fietsersbond for NL,
-- etc.) to be modeled without distorting the local-first defaults.
-- Filtering happens at the SQL level in AncestorRegions; see
-- internal/store/postgres/queries/lookup.sql.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE regions DROP CONSTRAINT regions_scope_tier_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE regions ADD CONSTRAINT regions_scope_tier_check
    CHECK (scope_tier IN ('local','regional','national'));
-- +goose StatementEnd

-- +goose Down

-- Refuse the downgrade if any national rows exist — would otherwise be
-- a silent data-loss operation (the CHECK addition would fail and roll
-- back, but we'd rather make the failure mode explicit and actionable).
-- Operator should DELETE national-scope rows explicitly before downgrade.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM regions WHERE scope_tier = 'national') THEN
        RAISE EXCEPTION 'Cannot downgrade 0003: regions.scope_tier=national rows exist (delete them first)';
    END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE regions DROP CONSTRAINT regions_scope_tier_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE regions ADD CONSTRAINT regions_scope_tier_check
    CHECK (scope_tier IN ('local','regional'));
-- +goose StatementEnd
