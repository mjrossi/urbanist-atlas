-- +goose Up
-- +goose StatementBegin

-- coverage_gaps records sampled empty-result lookups and searches — the
-- "which ZIP / which query returns nothing?" editorial signal. This is
-- the one place raw user input is persisted (the privacy bar's "sampled
-- empties" exception): only EMPTY-result requests land here, sampled and
-- row-capped via the recorder + PruneCoverageGaps. Non-empty traffic
-- stays aggregate-only in Prometheus.
CREATE TABLE coverage_gaps (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    kind       TEXT NOT NULL CHECK (kind IN ('lookup','search')),
    country    TEXT NOT NULL DEFAULT '',   -- '' for searches (no country axis)
    input      TEXT NOT NULL,              -- normalized postal code OR search query
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX coverage_gaps_created
    ON coverage_gaps(created_at DESC, id DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS coverage_gaps_created;
DROP TABLE IF EXISTS coverage_gaps;
-- +goose StatementEnd
