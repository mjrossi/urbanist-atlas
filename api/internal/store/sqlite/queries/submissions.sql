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
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: ListSubmissionsByStatus :many
SELECT
    id, public_id, payload_json,
    submitter_name, submitter_email, submitter_note,
    status, rejection_reason,
    created_at, processed_at,
    promotion_pr_url, promotion_error
FROM submissions
WHERE status = ?
ORDER BY created_at DESC, id DESC
LIMIT ?;

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
