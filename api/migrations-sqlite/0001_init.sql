-- +goose Up
-- +goose StatementBegin

CREATE TABLE submissions (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id        TEXT NOT NULL UNIQUE,
    payload_json     TEXT NOT NULL,
    submitter_name   TEXT NOT NULL DEFAULT '',
    submitter_email  TEXT NOT NULL DEFAULT '',
    submitter_note   TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','approved','rejected')),
    rejection_reason TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    processed_at     TEXT,
    promotion_pr_url TEXT NOT NULL DEFAULT '',
    promotion_error  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX submissions_status_created
    ON submissions(status, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS submissions_status_created;
DROP TABLE IF EXISTS submissions;
-- +goose StatementEnd
