-- +goose Up
-- +goose StatementBegin

-- usage_daily holds per-day aggregate counts of content popularity and
-- lookup outcomes — the durable record behind the monthly usage digest.
--
-- Aggregating by day is what makes per-slug popularity affordable here
-- when it was rejected as a Prometheus label (unbounded cardinality;
-- see the 2026-06-08 observability spec's D3). One row per
-- (day, kind, bucket_key) caps growth at the number of DISTINCT SLUGS
-- SERVED per day.
--
-- That bound only holds because callers write canonical slugs and never
-- raw request input: a 404 slug is deliberately not recorded (see the
-- notes in httpapi/regions.go and orgs.go), and internal/usage caps key
-- length and distinct buffered keys as defense in depth. Without those,
-- any caller could mint unbounded rows here — on the same 1 GiB volume
-- the submission queue writes to.
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

-- No secondary index: on a WITHOUT ROWID table the PRIMARY KEY
-- (day, kind, bucket_key) IS the b-tree, and (day, kind) is a strict
-- prefix of it, so the range scans below already plan as
-- "SEARCH usage_daily USING PRIMARY KEY (day>? AND day<?)". A separate
-- index could never be chosen (it does not cover bucket_key or count)
-- and would cost a second b-tree write on every flushed row.

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS usage_daily;
-- +goose StatementEnd
