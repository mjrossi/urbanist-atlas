// Package sqlite is the project's only writable store: the submission
// queue, coverage gaps and usage counts, in one SQLite database. Regions,
// orgs and postal codes stay in the file-backed atlas.MemStore. It uses
// the pure-Go modernc.org/sqlite driver and applies goose migrations from
// the embedded api/migrations-sqlite FS.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // pure-Go driver registered as "sqlite"

	migrations "github.com/mjrossi/urbanist-atlas/api/migrations-sqlite"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas/idgen"

	sqlitegen "github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite/gen"
)

// sqliteTimeFormat matches the strftime() DEFAULTs in the migrations, so
// timestamps sort and round-trip as TEXT.
const sqliteTimeFormat = "2006-01-02T15:04:05.000Z"

// Store wraps a *sql.DB with the sqlc-generated Queries and the
// pkg/atlas SubmissionStore contract.
type Store struct {
	db      *sql.DB
	q       *sqlitegen.Queries
	idgen   idgen.Generator
	nowFunc func() time.Time
}

// Open opens (or creates) the SQLite DB at path, which may be a file
// path, `:memory:` or `file::memory:?cache=shared`; the PRAGMA query
// string is appended here. Open does not migrate: call Migrate.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite.Open: empty path")
	}

	// PRAGMAs go in the DSN: WAL for write throughput, foreign_keys because
	// SQLite defaults it off, busy_timeout so lock contention waits rather
	// than erroring.
	dsn := dsnWithPragmas(path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite.Open: %w", err)
	}
	// SQLite is single-writer; one connection avoids SQLITE_BUSY pile-ups.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// sql.Open is lazy; ping now so a bad path fails at boot, not on the
	// first query.
	pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite.Open: ping: %w", err)
	}

	return &Store{
		db:      db,
		q:       sqlitegen.New(db),
		idgen:   idgen.NewUUIDv7(),
		nowFunc: func() time.Time { return time.Now().UTC() },
	}, nil
}

// Close releases the underlying *sql.DB.
func (s *Store) Close() error { return s.db.Close() }

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.db }

// SetClock overrides the time source, for deterministic tests.
func (s *Store) SetClock(now func() time.Time) {
	if now != nil {
		s.nowFunc = now
	}
}

// SetIDGenerator overrides the public-ID source, for tests.
func (s *Store) SetIDGenerator(g idgen.Generator) {
	if g != nil {
		s.idgen = g
	}
}

// Migrate applies any unapplied embedded migrations. Safe to call on
// every boot; goose's bookkeeping table tracks what has already run.
func (s *Store) Migrate(ctx context.Context) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("sqlite.Migrate: set dialect: %w", err)
	}
	// goose's dot-directory argument is relative to the BaseFS root.
	if err := goose.UpContext(ctx, s.db, "."); err != nil {
		return fmt.Errorf("sqlite.Migrate: up: %w", err)
	}
	return nil
}

// Create implements atlas.SubmissionStore.
func (s *Store) Create(ctx context.Context, in atlas.NewSubmissionInput) (atlas.Submission, error) {
	payload, err := json.Marshal(in.Payload)
	if err != nil {
		return atlas.Submission{}, fmt.Errorf("sqlite.Create: marshal payload: %w", err)
	}
	publicID, err := s.idgen()
	if err != nil {
		return atlas.Submission{}, fmt.Errorf("sqlite.Create: idgen: %w", err)
	}
	row, err := s.q.CreateSubmission(ctx, sqlitegen.CreateSubmissionParams{
		PublicID:       publicID,
		PayloadJson:    string(payload),
		SubmitterName:  in.SubmitterName,
		SubmitterEmail: in.SubmitterEmail,
		SubmitterNote:  in.SubmitterNote,
		CreatedAt:      s.nowFunc().Format(sqliteTimeFormat),
	})
	if err != nil {
		return atlas.Submission{}, fmt.Errorf("sqlite.Create: insert: %w", err)
	}
	return rowToSubmission(row)
}

// Get implements atlas.SubmissionStore.
func (s *Store) Get(ctx context.Context, publicID string) (atlas.Submission, error) {
	row, err := s.q.GetSubmissionByPublicID(ctx, publicID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return atlas.Submission{}, atlas.ErrSubmissionNotFound
		}
		return atlas.Submission{}, fmt.Errorf("sqlite.Get: %w", err)
	}
	return rowToSubmission(row)
}

// List implements atlas.SubmissionStore: ListPage without the cursor.
func (s *Store) List(ctx context.Context, q atlas.ListSubmissionsQuery) ([]atlas.Submission, error) {
	page, err := s.ListPage(ctx, q)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// ListPage implements atlas.SubmissionStore.
func (s *Store) ListPage(ctx context.Context, q atlas.ListSubmissionsQuery) (atlas.ListSubmissionsPage, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	// Fetch one extra row to decide whether there's a next page.
	fetch := limit + 1

	cursorTS, cursorID, hasCursor, err := decodeCursor(q.Cursor)
	if err != nil {
		return atlas.ListSubmissionsPage{}, err
	}

	var rows []sqlitegen.Submission
	switch {
	case q.Status == "" && !hasCursor:
		rows, err = s.q.ListSubmissionsAll(ctx, int64(fetch))
	case q.Status == "" && hasCursor:
		rows, err = s.q.ListSubmissionsAllAfter(ctx, sqlitegen.ListSubmissionsAllAfterParams{
			CursorCreatedAt: cursorTS,
			CursorPublicID:  cursorID,
			RowLimit:        int64(fetch),
		})
	case q.Status != "" && !hasCursor:
		rows, err = s.q.ListSubmissionsByStatus(ctx, sqlitegen.ListSubmissionsByStatusParams{
			Status: string(q.Status),
			Limit:  int64(fetch),
		})
	default:
		rows, err = s.q.ListSubmissionsByStatusAfter(ctx, sqlitegen.ListSubmissionsByStatusAfterParams{
			Status:          string(q.Status),
			CursorCreatedAt: cursorTS,
			CursorPublicID:  cursorID,
			RowLimit:        int64(fetch),
		})
	}
	if err != nil {
		return atlas.ListSubmissionsPage{}, fmt.Errorf("sqlite.ListPage: %w", err)
	}

	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next = encodeCursor(last.CreatedAt, last.PublicID)
		rows = rows[:limit]
	}

	out := make([]atlas.Submission, 0, len(rows))
	for _, r := range rows {
		sub, err := rowToSubmission(r)
		if err != nil {
			return atlas.ListSubmissionsPage{}, err
		}
		out = append(out, sub)
	}
	return atlas.ListSubmissionsPage{Items: out, NextCursor: next}, nil
}

// encodeCursor packs (created_at, public_id) into an opaque base64url
// cursor; callers pass back only what ListPage emitted.
func encodeCursor(createdAt, publicID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "|" + publicID))
}

// decodeCursor reverses encodeCursor, returning atlas.ErrInvalidCursor on
// malformed input and hasCursor=false for an empty cursor.
func decodeCursor(cursor string) (createdAt, publicID string, hasCursor bool, err error) {
	if cursor == "" {
		return "", "", false, nil
	}
	raw, derr := base64.RawURLEncoding.DecodeString(cursor)
	if derr != nil {
		return "", "", false, atlas.ErrInvalidCursor
	}
	sep := strings.IndexByte(string(raw), '|')
	if sep <= 0 || sep == len(raw)-1 {
		return "", "", false, atlas.ErrInvalidCursor
	}
	return string(raw[:sep]), string(raw[sep+1:]), true, nil
}

// Approve implements atlas.SubmissionStore.
func (s *Store) Approve(ctx context.Context, publicID string) (atlas.Submission, error) {
	row, err := s.q.ApproveSubmission(ctx, sqlitegen.ApproveSubmissionParams{
		ProcessedAt: sql.NullString{String: s.nowFunc().Format(sqliteTimeFormat), Valid: true},
		PublicID:    publicID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return atlas.Submission{}, s.notPendingOrNotFound(ctx, publicID)
		}
		return atlas.Submission{}, fmt.Errorf("sqlite.Approve: %w", err)
	}
	return rowToSubmission(row)
}

// Reject implements atlas.SubmissionStore.
func (s *Store) Reject(ctx context.Context, publicID, reason string) (atlas.Submission, error) {
	row, err := s.q.RejectSubmission(ctx, sqlitegen.RejectSubmissionParams{
		RejectionReason: reason,
		ProcessedAt:     sql.NullString{String: s.nowFunc().Format(sqliteTimeFormat), Valid: true},
		PublicID:        publicID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return atlas.Submission{}, s.notPendingOrNotFound(ctx, publicID)
		}
		return atlas.Submission{}, fmt.Errorf("sqlite.Reject: %w", err)
	}
	return rowToSubmission(row)
}

// AttachPromotionResult implements atlas.SubmissionStore. The existence
// check and the UPDATE share a transaction so a concurrent delete can't
// land between them; the single connection makes that unreachable today,
// but it stays correct if the cap is lifted.
func (s *Store) AttachPromotionResult(ctx context.Context, publicID, prURL, prErr string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite.AttachPromotionResult: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	// The contract promises ErrSubmissionNotFound for an unknown id.
	if _, err := qtx.SubmissionStatusByPublicID(ctx, publicID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return atlas.ErrSubmissionNotFound
		}
		return fmt.Errorf("sqlite.AttachPromotionResult: status check: %w", err)
	}
	if err := qtx.AttachPromotionResult(ctx, sqlitegen.AttachPromotionResultParams{
		PromotionPrUrl: prURL,
		PromotionError: prErr,
		PublicID:       publicID,
	}); err != nil {
		return fmt.Errorf("sqlite.AttachPromotionResult: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite.AttachPromotionResult: commit: %w", err)
	}
	return nil
}

// notPendingOrNotFound explains an ErrNoRows from an UPDATE … WHERE
// status='pending': a row that exists wasn't pending (409); otherwise
// it is missing (404).
func (s *Store) notPendingOrNotFound(ctx context.Context, publicID string) error {
	if _, err := s.q.SubmissionStatusByPublicID(ctx, publicID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return atlas.ErrSubmissionNotFound
		}
		return err
	}
	return atlas.ErrSubmissionNotPending
}

func rowToSubmission(r sqlitegen.Submission) (atlas.Submission, error) {
	var payload atlas.SubmissionPayload
	if err := json.Unmarshal([]byte(r.PayloadJson), &payload); err != nil {
		return atlas.Submission{}, fmt.Errorf("sqlite: row %s: unmarshal payload: %w", r.PublicID, err)
	}
	createdAt, err := parseSQLiteTime(r.CreatedAt)
	if err != nil {
		return atlas.Submission{}, fmt.Errorf("sqlite: row %s: parse created_at: %w", r.PublicID, err)
	}
	var processedAt *time.Time
	if r.ProcessedAt.Valid && r.ProcessedAt.String != "" {
		t, err := parseSQLiteTime(r.ProcessedAt.String)
		if err != nil {
			return atlas.Submission{}, fmt.Errorf("sqlite: row %s: parse processed_at: %w", r.PublicID, err)
		}
		processedAt = &t
	}
	return atlas.Submission{
		PublicID:        r.PublicID,
		Payload:         payload,
		SubmitterName:   r.SubmitterName,
		SubmitterEmail:  r.SubmitterEmail,
		SubmitterNote:   r.SubmitterNote,
		Status:          atlas.SubmissionStatus(r.Status),
		CreatedAt:       createdAt,
		ProcessedAt:     processedAt,
		PromotionPRURL:  r.PromotionPrUrl,
		PromotionError:  r.PromotionError,
		RejectionReason: r.RejectionReason,
	}, nil
}

// parseSQLiteTime accepts sqliteTimeFormat (the migrations' strftime
// DEFAULT) and RFC3339.
func parseSQLiteTime(s string) (time.Time, error) {
	for _, layout := range []string{sqliteTimeFormat, time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time format %q", s)
}

// dsnWithPragmas appends the PRAGMAs to a SQLite DSN; modernc reads
// repeated _pragma= parameters.
func dsnWithPragmas(path string) string {
	pragmas := []string{
		"_pragma=journal_mode(WAL)",
		"_pragma=foreign_keys(on)",
		"_pragma=busy_timeout(5000)",
	}
	sep := "?"
	if strings.Contains(path, "?") {
		// Caller already added a query string (e.g. `file::memory:?cache=shared`).
		sep = "&"
	}
	return path + sep + strings.Join(escapePragmas(pragmas), "&")
}

func escapePragmas(in []string) []string {
	out := make([]string, len(in))
	for i, p := range in {
		// Escape key and value both so the pair is a well-formed query
		// parameter whatever it holds; values contain '(' and ')'.
		eq := strings.IndexByte(p, '=')
		if eq < 0 {
			out[i] = url.QueryEscape(p)
			continue
		}
		out[i] = url.QueryEscape(p[:eq]) + "=" + url.QueryEscape(p[eq+1:])
	}
	return out
}
