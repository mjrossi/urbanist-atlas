package sqlite_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas/idgen"
)

func newTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	// Each test gets a fresh in-memory DB. The shared-cache URI plus
	// MaxOpenConns(1) (set by Open) keeps the schema reachable from
	// every query within the test.
	s, err := sqlite.Open("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

func TestStore_Ping(t *testing.T) {
	// Alive connection pings cleanly. This is the readiness-probe path
	// (/readyz) — *sqlite.Store satisfies the httpapi pinger contract.
	s := newTestStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping on open store: %v", err)
	}

	// A separately-opened store (not the t.Cleanup-managed one) so we can
	// close it and confirm Ping reports the dead connection.
	closed, err := sqlite.Open("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := closed.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := closed.Ping(context.Background()); err == nil {
		t.Fatal("Ping on closed store: want error, got nil")
	}
}

func samplePayload() atlas.SubmissionPayload {
	return atlas.SubmissionPayload{
		Name:        "Brooklyn Greenways",
		ShortDesc:   "Volunteers expanding the borough's protected-lane network.",
		WebsiteURL:  "https://example.org/brooklyn-greenways",
		ContactURL:  "https://example.org/brooklyn-greenways/contact",
		Tags:        []string{"cycling", "grassroots"},
		RegionSlugs: []string{"brooklyn-ny"},
	}
}

func TestCreateAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, atlas.NewSubmissionInput{
		Payload:        samplePayload(),
		SubmitterName:  "Jane",
		SubmitterEmail: "jane@example.org",
		SubmitterNote:  "Already coordinating with DOT.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.PublicID == "" {
		t.Fatal("Create: PublicID empty")
	}
	if created.Status != atlas.SubmissionPending {
		t.Fatalf("Create: status = %q, want pending", created.Status)
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("Create: CreatedAt zero")
	}
	if created.ProcessedAt != nil {
		t.Fatalf("Create: ProcessedAt = %v, want nil", created.ProcessedAt)
	}

	got, err := s.Get(ctx, created.PublicID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Payload.Name != "Brooklyn Greenways" {
		t.Fatalf("Get: payload not round-tripped, got %+v", got.Payload)
	}
	if len(got.Payload.Tags) != 2 || got.Payload.Tags[0] != "cycling" {
		t.Fatalf("Get: tags lost, got %+v", got.Payload.Tags)
	}
}

func TestGet_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get(context.Background(), "does-not-exist")
	if !errors.Is(err, atlas.ErrSubmissionNotFound) {
		t.Fatalf("Get unknown id: err = %v, want ErrSubmissionNotFound", err)
	}
}

func TestList_FilterAndOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert three with deterministic ordering by injecting an
	// increasing clock so created_at sorts predictably.
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	tick := 0
	s.SetClock(func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Second)
	})

	for range 3 {
		if _, err := s.Create(ctx, atlas.NewSubmissionInput{Payload: samplePayload()}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	all, err := s.List(ctx, atlas.ListSubmissionsQuery{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List all: len = %d, want 3", len(all))
	}
	// Newest first.
	if !all[0].CreatedAt.After(all[1].CreatedAt) || !all[1].CreatedAt.After(all[2].CreatedAt) {
		t.Fatalf("List: not ordered newest-first: %+v", []time.Time{all[0].CreatedAt, all[1].CreatedAt, all[2].CreatedAt})
	}

	pending, err := s.List(ctx, atlas.ListSubmissionsQuery{Status: atlas.SubmissionPending, Limit: 2})
	if err != nil {
		t.Fatalf("List pending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("List pending limit=2: len = %d, want 2", len(pending))
	}

	approvedOnly, err := s.List(ctx, atlas.ListSubmissionsQuery{Status: atlas.SubmissionApproved})
	if err != nil {
		t.Fatalf("List approved: %v", err)
	}
	if len(approvedOnly) != 0 {
		t.Fatalf("List approved (none yet): len = %d, want 0", len(approvedOnly))
	}
}

func TestApprove_FlipsStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sub, err := s.Create(ctx, atlas.NewSubmissionInput{Payload: samplePayload()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	approved, err := s.Approve(ctx, sub.PublicID)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Status != atlas.SubmissionApproved {
		t.Fatalf("Approve: status = %q, want approved", approved.Status)
	}
	if approved.ProcessedAt == nil {
		t.Fatal("Approve: ProcessedAt nil, want set")
	}

	// Second approval must be 409, not 404.
	_, err = s.Approve(ctx, sub.PublicID)
	if !errors.Is(err, atlas.ErrSubmissionNotPending) {
		t.Fatalf("second Approve: err = %v, want ErrSubmissionNotPending", err)
	}
}

func TestApprove_UnknownIDReturns404(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Approve(context.Background(), "no-such-id")
	if !errors.Is(err, atlas.ErrSubmissionNotFound) {
		t.Fatalf("Approve unknown: err = %v, want ErrSubmissionNotFound", err)
	}
}

func TestReject_Persists(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sub, err := s.Create(ctx, atlas.NewSubmissionInput{Payload: samplePayload()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	rej, err := s.Reject(ctx, sub.PublicID, "duplicate of existing org")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if rej.Status != atlas.SubmissionRejected {
		t.Fatalf("Reject: status = %q, want rejected", rej.Status)
	}
	if rej.RejectionReason != "duplicate of existing org" {
		t.Fatalf("Reject: reason = %q, want persisted", rej.RejectionReason)
	}

	// Second rejection must be 409.
	_, err = s.Reject(ctx, sub.PublicID, "again")
	if !errors.Is(err, atlas.ErrSubmissionNotPending) {
		t.Fatalf("second Reject: err = %v, want ErrSubmissionNotPending", err)
	}
}

func TestAttachPromotionResult(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sub, err := s.Create(ctx, atlas.NewSubmissionInput{Payload: samplePayload()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.Approve(ctx, sub.PublicID); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// Happy path: PR URL.
	if err := s.AttachPromotionResult(ctx, sub.PublicID, "https://github.com/mjrossi/urbanist-atlas/pull/99", ""); err != nil {
		t.Fatalf("AttachPromotionResult: %v", err)
	}
	got, err := s.Get(ctx, sub.PublicID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.HasSuffix(got.PromotionPRURL, "/pull/99") {
		t.Fatalf("PromotionPRURL = %q", got.PromotionPRURL)
	}

	// Error path overwrites cleanly.
	if err := s.AttachPromotionResult(ctx, sub.PublicID, "", "github 502"); err != nil {
		t.Fatalf("AttachPromotionResult error update: %v", err)
	}
	got, err = s.Get(ctx, sub.PublicID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PromotionError != "github 502" {
		t.Fatalf("PromotionError = %q", got.PromotionError)
	}
}

func TestAttachPromotionResult_UnknownID(t *testing.T) {
	s := newTestStore(t)
	err := s.AttachPromotionResult(context.Background(), "no-such-id", "https://x", "")
	if !errors.Is(err, atlas.ErrSubmissionNotFound) {
		t.Fatalf("err = %v, want ErrSubmissionNotFound", err)
	}
}

// TestListPage_CursorContinuity pins issue #30: the keyset cursor
// (created_at, public_id) must line up exactly with the queries'
// ORDER BY created_at DESC, public_id DESC. Paging through with the
// emitted NextCursor must visit every row exactly once, in strict
// newest-first order, with no duplicates or gaps across page
// boundaries — including rows that share a created_at (the composite
// public_id tiebreak is what the cursor relies on).
func TestListPage_CursorContinuity(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// 25 rows, but only 5 distinct created_at values (5 rows per second)
	// so the public_id tiebreak in the cursor is genuinely exercised.
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	tick := -1
	s.SetClock(func() time.Time {
		tick++
		return base.Add(time.Duration(tick/5) * time.Second)
	})

	const total = 25
	for range total {
		if _, err := s.Create(ctx, atlas.NewSubmissionInput{Payload: samplePayload()}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	seen := make(map[string]bool, total)
	var ordered []atlas.Submission
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > total {
			t.Fatal("pagination did not terminate (cursor never emptied)")
		}
		page, err := s.ListPage(ctx, atlas.ListSubmissionsQuery{Limit: 7, Cursor: cursor})
		if err != nil {
			t.Fatalf("ListPage page %d: %v", pages, err)
		}
		for _, sub := range page.Items {
			if seen[sub.PublicID] {
				t.Fatalf("duplicate row across page boundary: %s", sub.PublicID)
			}
			seen[sub.PublicID] = true
			ordered = append(ordered, sub)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	if len(ordered) != total {
		t.Fatalf("paged %d rows, want %d (gap or early stop)", len(ordered), total)
	}
	// Strict newest-first across the whole concatenation: each row must
	// be <= the previous by (created_at, public_id) DESC.
	for i := 1; i < len(ordered); i++ {
		prev, cur := ordered[i-1], ordered[i]
		if cur.CreatedAt.After(prev.CreatedAt) {
			t.Fatalf("row %d created_at %v is newer than predecessor %v — order broke across pages",
				i, cur.CreatedAt, prev.CreatedAt)
		}
		if cur.CreatedAt.Equal(prev.CreatedAt) && cur.PublicID >= prev.PublicID {
			t.Fatalf("row %d public_id %s !< predecessor %s within same created_at — tiebreak/cursor mismatch",
				i, cur.PublicID, prev.PublicID)
		}
	}
}

// TestListPage_InvalidCursor pins that a cursor never emitted by
// ListPage is rejected with ErrInvalidCursor (a 400, not a silent
// full-table scan).
func TestListPage_InvalidCursor(t *testing.T) {
	s := newTestStore(t)
	_, err := s.ListPage(context.Background(), atlas.ListSubmissionsQuery{Cursor: "not-valid-base64-$$$"})
	if !errors.Is(err, sqlite.ErrInvalidCursor) {
		t.Fatalf("err = %v, want ErrInvalidCursor", err)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	s := newTestStore(t)
	// newTestStore already ran Migrate once. A second call must be a
	// no-op, not an error.
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestCreate_InjectedID(t *testing.T) {
	s := newTestStore(t)
	canned := "0192f6c0-1c2c-7000-9000-000000000001"
	s.SetIDGenerator(idgen.Generator(func() (string, error) { return canned, nil }))
	got, err := s.Create(context.Background(), atlas.NewSubmissionInput{Payload: samplePayload()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.PublicID != canned {
		t.Fatalf("PublicID = %q, want %q", got.PublicID, canned)
	}
}
