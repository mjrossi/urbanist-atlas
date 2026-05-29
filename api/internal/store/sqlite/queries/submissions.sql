-- name: CreateSubmission :one
INSERT INTO submissions (
    public_id,
    payload_json,
    submitter_name,
    submitter_email,
    submitter_note,
    created_at
) VALUES (?, ?, ?, ?, ?, ?)
RETURNING
    id, public_id, payload_json,
    submitter_name, submitter_email, submitter_note,
    status, rejection_reason,
    created_at, processed_at,
    promotion_pr_url, promotion_error;

-- name: GetSubmissionByPublicID :one
SELECT
    id, public_id, payload_json,
    submitter_name, submitter_email, submitter_note,
    status, rejection_reason,
    created_at, processed_at,
    promotion_pr_url, promotion_error
FROM submissions
WHERE public_id = ?;

-- name: ListSubmissionsAll :many
SELECT
    id, public_id, payload_json,
    submitter_name, submitter_email, submitter_note,
    status, rejection_reason,
    created_at, processed_at,
    promotion_pr_url, promotion_error
FROM submissions
ORDER BY created_at DESC, public_id DESC
LIMIT ?;

-- name: ListSubmissionsAllAfter :many
-- Keyset pagination after (created_at, public_id). The composite
-- predicate gives a stable cursor even when multiple rows share a ms.
SELECT
    id, public_id, payload_json,
    submitter_name, submitter_email, submitter_note,
    status, rejection_reason,
    created_at, processed_at,
    promotion_pr_url, promotion_error
FROM submissions
WHERE created_at < sqlc.arg(cursor_created_at)
   OR (created_at = sqlc.arg(cursor_created_at)
       AND public_id < sqlc.arg(cursor_public_id))
ORDER BY created_at DESC, public_id DESC
LIMIT sqlc.arg(row_limit);

-- name: ListSubmissionsByStatus :many
SELECT
    id, public_id, payload_json,
    submitter_name, submitter_email, submitter_note,
    status, rejection_reason,
    created_at, processed_at,
    promotion_pr_url, promotion_error
FROM submissions
WHERE status = ?
ORDER BY created_at DESC, public_id DESC
LIMIT ?;

-- name: ListSubmissionsByStatusAfter :many
SELECT
    id, public_id, payload_json,
    submitter_name, submitter_email, submitter_note,
    status, rejection_reason,
    created_at, processed_at,
    promotion_pr_url, promotion_error
FROM submissions
WHERE status = sqlc.arg(status)
  AND (created_at < sqlc.arg(cursor_created_at)
       OR (created_at = sqlc.arg(cursor_created_at)
           AND public_id < sqlc.arg(cursor_public_id)))
ORDER BY created_at DESC, public_id DESC
LIMIT sqlc.arg(row_limit);

-- name: ApproveSubmission :one
UPDATE submissions
   SET status       = 'approved',
       processed_at = ?
 WHERE public_id = ?
   AND status    = 'pending'
RETURNING
    id, public_id, payload_json,
    submitter_name, submitter_email, submitter_note,
    status, rejection_reason,
    created_at, processed_at,
    promotion_pr_url, promotion_error;

-- name: RejectSubmission :one
UPDATE submissions
   SET status           = 'rejected',
       rejection_reason = ?,
       processed_at     = ?
 WHERE public_id = ?
   AND status    = 'pending'
RETURNING
    id, public_id, payload_json,
    submitter_name, submitter_email, submitter_note,
    status, rejection_reason,
    created_at, processed_at,
    promotion_pr_url, promotion_error;

-- name: AttachPromotionResult :exec
UPDATE submissions
   SET promotion_pr_url = ?,
       promotion_error  = ?
 WHERE public_id = ?;

-- name: SubmissionStatusByPublicID :one
SELECT status
FROM submissions
WHERE public_id = ?;
