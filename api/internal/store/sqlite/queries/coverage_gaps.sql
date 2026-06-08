-- name: InsertCoverageGap :exec
INSERT INTO coverage_gaps (kind, country, input, created_at)
VALUES (sqlc.arg(kind), sqlc.arg(country), sqlc.arg(input), sqlc.arg(created_at));

-- name: ListCoverageGaps :many
SELECT id, kind, country, input, created_at
FROM coverage_gaps
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: PruneCoverageGaps :exec
-- Keep only the newest sqlc.arg(keep) rows; delete the rest. Called
-- opportunistically after each insert so the table stays bounded.
DELETE FROM coverage_gaps
WHERE id NOT IN (
    SELECT id FROM coverage_gaps
    ORDER BY created_at DESC, id DESC
    LIMIT sqlc.arg(keep)
);
