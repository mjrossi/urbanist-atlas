-- Queries for usage_daily, the monthly-digest rollup table.
--
-- EDITING NOTE: sqlc's SQLite lexer scans these comments, and an
-- unpaired apostrophe in one (e.g. writing "LIMIT'd") opens a string
-- literal that swallows the rest of the file and fails the whole
-- package with a misleading "extraneous input" parse error pointing at
-- the SELECT below. Keep apostrophes out of comments here, or pair
-- them.

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

-- name: ListUsageTotals :many
-- Totals per (kind, bucket_key) across the whole day range -- the shape
-- the monthly digest actually wants.
--
-- Why this exists alongside ListUsage: ListUsage returns one row per
-- DAY, so a month of traffic is days x kinds x keys rows, and a capped
-- read both truncates the month and ranks by single-day count, which
-- buries a slug with steady daily traffic under one that spiked once.
-- Grouping first collapses the row count by roughly 31x and makes the
-- top-N ordering mean "most viewed over the range".
--
-- CAST pins the SUM to INTEGER; SQLite would otherwise leave the column
-- type open and sqlc would generate an interface{} field.
SELECT kind, bucket_key, CAST(SUM(count) AS INTEGER) AS total
FROM usage_daily
WHERE day >= sqlc.arg(from_day)
  AND day <= sqlc.arg(to_day)
GROUP BY kind, bucket_key
ORDER BY total DESC, kind ASC, bucket_key ASC
LIMIT sqlc.arg(row_limit);

-- name: ListUsageTotalsByKind :many
-- As ListUsageTotals, restricted to one bucket kind. Split for the same
-- sqlc named-argument reason as ListUsage/ListUsageByKind above.
SELECT kind, bucket_key, CAST(SUM(count) AS INTEGER) AS total
FROM usage_daily
WHERE day >= sqlc.arg(from_day)
  AND day <= sqlc.arg(to_day)
  AND kind = sqlc.arg(kind)
GROUP BY kind, bucket_key
ORDER BY total DESC, bucket_key ASC
LIMIT sqlc.arg(row_limit);
