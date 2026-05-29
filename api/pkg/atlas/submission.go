package atlas

import (
	"context"
	"errors"
	"time"
)

// ErrSubmissionNotFound is returned by SubmissionStore methods when no
// row matches the supplied public ID. The HTTP layer maps this to a
// 404 problem document.
var ErrSubmissionNotFound = errors.New("atlas: submission not found")

// ErrSubmissionNotPending is returned by ApproveSubmission and
// RejectSubmission when the target row is not in pending state.
// The HTTP layer maps this to a 409 problem document.
var ErrSubmissionNotPending = errors.New("atlas: submission not pending")

// SubmissionStatus mirrors the wire enum in openapi.yaml.
type SubmissionStatus string

const (
	SubmissionPending  SubmissionStatus = "pending"
	SubmissionApproved SubmissionStatus = "approved"
	SubmissionRejected SubmissionStatus = "rejected"
)

// SubmissionPayload is the proposed organization data, mirroring the
// `[[org]]` TOML shape (slug intentionally omitted — moderators
// finalize it when approving and the PR worker reuses the form
// payload's website-derived default).
type SubmissionPayload struct {
	Name        string   `json:"name"`
	ShortDesc   string   `json:"short_desc"`
	WebsiteURL  string   `json:"website_url"`
	ContactURL  string   `json:"contact_url,omitempty"`
	Tags        []string `json:"tags"`
	RegionSlugs []string `json:"region_slugs"`
}

// Submission is a queued or processed public submission. The wire
// contract exposes only the UUIDv7 PublicID; the database's INTEGER
// primary key never leaves the storage layer.
type Submission struct {
	PublicID        string
	Payload         SubmissionPayload
	SubmitterName   string
	SubmitterEmail  string
	SubmitterNote   string
	Status          SubmissionStatus
	CreatedAt       time.Time
	ProcessedAt     *time.Time
	PromotionPRURL  string
	PromotionError  string
	RejectionReason string
}

// NewSubmissionInput is what callers pass to SubmissionStore.Create.
// The store generates PublicID + CreatedAt; callers supply only the
// user-provided fields.
type NewSubmissionInput struct {
	Payload        SubmissionPayload
	SubmitterName  string
	SubmitterEmail string
	SubmitterNote  string
}

// ListSubmissionsQuery filters and paginates SubmissionStore.List.
// Status="" means "all". Limit<=0 falls back to a sensible default
// (50); the store caps it at 200.
//
// Cursor is opaque to callers — pass the previous page's NextCursor
// to fetch the next page. Empty means "start at the newest row".
// The encoding is keyset on (created_at, public_id) so pagination is
// stable even when multiple rows share a millisecond timestamp.
type ListSubmissionsQuery struct {
	Status SubmissionStatus
	Limit  int
	Cursor string
}

// ListSubmissionsPage is a paginated slice of submissions plus the
// cursor a caller should pass to fetch the next page. NextCursor is
// empty when there are no more rows.
type ListSubmissionsPage struct {
	Items      []Submission
	NextCursor string
}

// SubmissionStore is the persistence seam for the public submission
// queue. Implementations must be safe for concurrent use.
type SubmissionStore interface {
	// Create inserts a new submission with status=pending, stamps the
	// server-generated UUIDv7 PublicID + CreatedAt, and returns the
	// hydrated row.
	Create(ctx context.Context, in NewSubmissionInput) (Submission, error)

	// Get returns the submission with the given public ID, or
	// ErrSubmissionNotFound.
	Get(ctx context.Context, publicID string) (Submission, error)

	// List returns submissions newest-first matching q. The bare slice
	// shape is preserved for callers that don't need pagination; use
	// ListPage when you need a follow-up cursor.
	List(ctx context.Context, q ListSubmissionsQuery) ([]Submission, error)

	// ListPage is List + the keyset cursor for the next page. Empty
	// NextCursor means no more rows.
	ListPage(ctx context.Context, q ListSubmissionsQuery) (ListSubmissionsPage, error)

	// Approve flips status pending→approved and stamps ProcessedAt.
	// Returns ErrSubmissionNotPending if the row has already been
	// processed; ErrSubmissionNotFound if the id is unknown.
	Approve(ctx context.Context, publicID string) (Submission, error)

	// Reject flips status pending→rejected, persists reason, and
	// stamps ProcessedAt. Same error contract as Approve.
	Reject(ctx context.Context, publicID, reason string) (Submission, error)

	// AttachPromotionResult records the GitHub PR worker's outcome on
	// an approved submission. prURL and prErr are mutually exclusive
	// in practice but the store does not enforce that — it writes
	// whatever the worker reports. Returns ErrSubmissionNotFound for
	// unknown IDs.
	AttachPromotionResult(ctx context.Context, publicID, prURL, prErr string) error
}
