-- name: UpsertUsageCount :exec
-- Accumulates rather than replaces: the recorder flushes deltas accrued
-- since the last flush, so repeated flushes on the same day must sum.
INSERT INTO usage_daily (day, kind, bucket_key, count)
VALUES (sqlc.arg(day), sqlc.arg(kind), sqlc.arg(bucket_key), sqlc.arg(count))
ON CONFLICT (day, kind, bucket_key)
DO UPDATE SET count = count + excluded.count;

-- name: ListUsage :many
-- Every kind in the day range, ordered by count DESC so a limited read
-- returns the top buckets rather than an arbitrary slice.
--
-- Split from ListUsageByKind rather than using one query with an
-- optional filter: the sqlc SQLite parser rejects a named argument
-- referenced twice, which is what the usual
-- "(arg = '' OR col = arg)" idiom requires.
SELECT day, kind, bucket_key, count
FROM usage_daily
WHERE day >= sqlc.arg(from_day)
  AND day <= sqlc.arg(to_day)
ORDER BY count DESC, day DESC, kind ASC, bucket_key ASC
LIMIT sqlc.arg(row_limit);

-- name: ListUsageByKind :many
-- As ListUsage, restricted to one bucket kind.
SELECT day, kind, bucket_key, count
FROM usage_daily
WHERE day >= sqlc.arg(from_day)
  AND day <= sqlc.arg(to_day)
  AND kind = sqlc.arg(kind)
ORDER BY count DESC, day DESC, bucket_key ASC
LIMIT sqlc.arg(row_limit);

-- name: PruneUsage :exec
-- Drops buckets older than the cutoff day, keeping the table bounded.
-- Called opportunistically after a flush.
DELETE FROM usage_daily WHERE day < sqlc.arg(cutoff_day);
