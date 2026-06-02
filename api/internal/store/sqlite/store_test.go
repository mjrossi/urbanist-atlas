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
