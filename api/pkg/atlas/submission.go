package atlas

import (
	"context"
	"errors"
	"time"
)

// ErrSubmissionNotFound is returned by SubmissionStore methods when no
// row matches the supplied public ID.
var ErrSubmissionNotFound = errors.New("atlas: submission not found")

// ErrSubmissionNotPending is returned by SubmissionStore.Approve and
// Reject when the target row is not pending.
var ErrSubmissionNotPending = errors.New("atlas: submission not pending")

// ErrInvalidCursor is returned by SubmissionStore.ListPage when the
// cursor is not one ListPage emitted.
var ErrInvalidCursor = errors.New("atlas: invalid submission cursor")

// ErrPromotionDisabled is returned when an approved submission cannot be
// queued for a promotion PR because no GitHub token is configured. Its
// text is persisted as the submission's promotion_error.
var ErrPromotionDisabled = errors.New("atlas: promotion worker disabled (no token configured)")

// SubmissionStatus mirrors the wire enum in openapi.yaml.
type SubmissionStatus string

const (
	SubmissionPending  SubmissionStatus = "pending"
	SubmissionApproved SubmissionStatus = "approved"
	SubmissionRejected SubmissionStatus = "rejected"
)

// SubmissionPayload is the proposed organization data, mirroring the
// `[[org]]` TOML shape minus the slug, which is assigned when the org is
// promoted into orgs.toml.
type SubmissionPayload struct {
	Name       string `json:"name"`
	ShortDesc  string `json:"short_desc"`
	WebsiteURL string `json:"website_url"`
	ContactURL string `json:"contact_url,omitempty"`
	// Tags is not on the wire, so it is nil for public submissions; an
	// editor assigns tags during PR review.
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

// NewSubmissionInput is the user-provided part of a Submission, passed
// to SubmissionStore.Create.
type NewSubmissionInput struct {
	Payload        SubmissionPayload
	SubmitterName  string
	SubmitterEmail string
	SubmitterNote  string
}

// ListSubmissionsQuery filters and paginates SubmissionStore.List.
// Status="" means all; Limit<=0 selects a default, and implementations
// cap it. Cursor is the previous page's NextCursor, or "" to start at
// the newest row.
type ListSubmissionsQuery struct {
	Status SubmissionStatus
	Limit  int
	Cursor string
}

// ListSubmissionsPage is one page of submissions. NextCursor is empty
// when there are no more rows.
type ListSubmissionsPage struct {
	Items      []Submission
	NextCursor string
}

// SubmissionStore is the persistence seam for the public submission
// queue. Implementations must be safe for concurrent use.
type SubmissionStore interface {
	// Create inserts a pending submission, stamping PublicID and
	// CreatedAt, and returns the stored row.
	Create(ctx context.Context, in NewSubmissionInput) (Submission, error)

	// Get returns the submission with the given public ID, or
	// ErrSubmissionNotFound.
	Get(ctx context.Context, publicID string) (Submission, error)

	// List returns submissions matching q, newest-first, without a
	// follow-up cursor.
	List(ctx context.Context, q ListSubmissionsQuery) ([]Submission, error)

	// ListPage is List plus the cursor for the next page.
	ListPage(ctx context.Context, q ListSubmissionsQuery) (ListSubmissionsPage, error)

	// Approve moves a pending submission to approved and stamps
	// ProcessedAt, or returns ErrSubmissionNotPending or
	// ErrSubmissionNotFound.
	Approve(ctx context.Context, publicID string) (Submission, error)

	// Reject moves a pending submission to rejected and persists reason,
	// with the same errors as Approve.
	Reject(ctx context.Context, publicID, reason string) (Submission, error)

	// AttachPromotionResult records the promotion PR's URL or error on a
	// submission, writing both as given, or returns ErrSubmissionNotFound.
	AttachPromotionResult(ctx context.Context, publicID, prURL, prErr string) error
}
