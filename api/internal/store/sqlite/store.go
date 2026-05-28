// Package sqlite is the SubmissionStore implementation backed by a
// SQLite database on disk (or in-memory for tests). It's the only
// writable store in the project — everything else (regions, orgs,
// postal codes) stays in the file-backed atlas.MemStore.
//
// The driver is modernc.org/sqlite (pure Go, no CGO). Migrations are
// applied via goose from the embedded api/migrations-sqlite FS, so
// the binary needs nothing on disk at boot beyond the DB file.
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

// ErrInvalidCursor is returned by List/ListPage when the supplied
// cursor isn't a value previously emitted by ListPage. The HTTP layer
// surfaces this as a 400 problem document.
var ErrInvalidCursor = errors.New("sqlite.List: invalid cursor")

// SQLite stores timestamps as TEXT in this format so they round-trip
// cleanly through strftime() defaults from the migration.
const sqliteTimeFormat = "2006-01-02T15:04:05.000Z"

// Store wraps a *sql.DB with the sqlc-generated Queries and the
// pkg/atlas SubmissionStore contract.
type Store struct {
	db      *sql.DB
	q       *sqlitegen.Queries
	idgen   idgen.Generator
	nowFunc func() time.Time
}

// Open opens (or creates) the SQLite DB at path and returns a Store.
// path may be a filesystem path, a `:memory:` URI, or
// `file::memory:?cache=shared`. The PRAGMA query string is appended
// automatically; callers should pass only the base path.
//
// Open does NOT apply migrations — call Migrate after Open so callers
// can decide when migrations run (typically at boot, never during
// tests against pre-seeded databases).
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite.Open: empty path")
	}

	// modernc's driver name is "sqlite" (not "sqlite3"). PRAGMAs go in
	// the URI's _pragma query — WAL for write throughput, foreign_keys
	// because the default is OFF and we want correct CASCADE behavior
	// if future schemas add FKs, busy_timeout so transient lock
	// contention waits instead of erroring.
	dsn := dsnWithPragmas(path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite.Open: %w", err)
	}
	// SQLite is single-writer; cap connections so callers don't
	// pile up SQLITE_BUSY waits on lock contention.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// sql.Open is lazy — the first query is what reveals a bad path or
	// a corrupted file. Ping eagerly so the boot-time error is clean
	// ("path does not exist") instead of leaking into the first
	// Migrate or Create call. 2s is generous for a local file open.
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

// DB returns the underlying *sql.DB. Useful for callers that need to
// run goose programmatically or for tests that want to inspect rows
// directly.
func (s *Store) DB() *sql.DB { return s.db }

// SetClock overrides the time source. Tests use this to make
// processed_at deterministic.
func (s *Store) SetClock(now func() time.Time) {
	if now != nil {
		s.nowFunc = now
	}
}

// SetIDGenerator overrides the public-ID source. Tests use this to
// inject canned UUIDv7 values.
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

// List implements atlas.SubmissionStore. It's a thin wrapper around
// ListPage that drops the cursor — preserved for callers (tests,
// internal code) that don't paginate.
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
// string. The format is deliberately stable but undocumented — callers
// must treat it as opaque and only pass back what ListPage emitted.
func encodeCursor(createdAt, publicID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "|" + publicID))
}

// decodeCursor returns the components of a cursor previously produced
// by encodeCursor, or ErrInvalidCursor on malformed input. Empty
// cursor means "no cursor" — returns hasCursor=false with no error.
func decodeCursor(cursor string) (createdAt, publicID string, hasCursor bool, err error) {
	if cursor == "" {
		return "", "", false, nil
	}
	raw, derr := base64.RawURLEncoding.DecodeString(cursor)
	if derr != nil {
		return "", "", false, ErrInvalidCursor
	}
	sep := strings.IndexByte(string(raw), '|')
	if sep <= 0 || sep == len(raw)-1 {
		return "", "", false, ErrInvalidCursor
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

// AttachPromotionResult implements atlas.SubmissionStore.
func (s *Store) AttachPromotionResult(ctx context.Context, publicID, prURL, prErr string) error {
	// We need to know whether the id exists so the worker can log a
	// loud error if approvals raced with a delete (not a current code
	// path, but the contract promises ErrSubmissionNotFound).
	if _, err := s.q.SubmissionStatusByPublicID(ctx, publicID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return atlas.ErrSubmissionNotFound
		}
		return fmt.Errorf("sqlite.AttachPromotionResult: status check: %w", err)
	}
	if err := s.q.AttachPromotionResult(ctx, sqlitegen.AttachPromotionResultParams{
		PromotionPrUrl: prURL,
		PromotionError: prErr,
		PublicID:       publicID,
	}); err != nil {
		return fmt.Errorf("sqlite.AttachPromotionResult: %w", err)
	}
	return nil
}

// notPendingOrNotFound disambiguates ErrNoRows on an UPDATE … WHERE
// status='pending'. If the row exists at all, it must have been in a
// non-pending state, so 409. Otherwise 404.
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

// parseSQLiteTime accepts the strftime('%Y-%m-%dT%H:%M:%fZ') format
// from the migration's DEFAULT as well as plain RFC3339 (what Go's
// Format produces) so values written by our code round-trip cleanly.
func parseSQLiteTime(s string) (time.Time, error) {
	for _, layout := range []string{sqliteTimeFormat, time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time format %q", s)
}

// dsnWithPragmas appends WAL/foreign_keys/busy_timeout PRAGMAs to a
// SQLite DSN. modernc's driver consumes _pragma= multiple times.
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
		// Pragmas are key=value where the value may contain '(' / ')'.
		// modernc accepts the raw form, but going through url.QueryEscape
		// for the value half keeps any future caller-supplied path safe.
		eq := strings.IndexByte(p, '=')
		if eq < 0 {
			out[i] = p
			continue
		}
		out[i] = p[:eq+1] + url.QueryEscape(p[eq+1:])
	}
	return out
}
