-- +goose Up
-- +goose StatementBegin

-- usage_daily holds per-day aggregate counts of content popularity and
-- lookup outcomes — the durable record behind the monthly usage digest.
--
-- Aggregating by day is what makes per-slug popularity affordable here
-- when it was rejected as a Prometheus label (unbounded cardinality;
-- see the 2026-06-08 observability spec's D3). One row per
-- (day, kind, bucket_key) caps growth at roughly the slug count per day.
--
-- PRIVACY: bucket_key holds only public content identifiers (region and
-- org slugs) or bounded enum values — never raw postal codes or search
-- queries. Raw user input is persisted ONLY in coverage_gaps, sampled,
-- per the 2026-06-08 D4 privacy bar.
CREATE TABLE usage_daily (
    day        TEXT    NOT NULL,           -- 'YYYY-MM-DD', UTC
    kind       TEXT    NOT NULL CHECK (kind IN (
                   'region_view','org_view','lookup',
                   'lookup_tier','lookup_result','lookup_country')),
    bucket_key TEXT    NOT NULL,           -- region/org slug, tier, result, or country
    count      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, kind, bucket_key)
) WITHOUT ROWID;

-- Serves the digest's range scan (day BETWEEN ? AND ?, optional kind).
CREATE INDEX usage_daily_day_kind ON usage_daily(day, kind);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS usage_daily_day_kind;
DROP TABLE IF EXISTS usage_daily;
-- +goose StatementEnd
