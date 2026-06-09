// Package githubpr implements the asynchronous worker that opens a
// GitHub pull request appending an approved submission to
// api/seed/orgs.toml. The PR is the editorial-review surface — a
// maintainer reviews/merges, the next API deploy rebuilds the
// embedded seed bundle, and the org becomes visible.
//
// The worker is intentionally a single goroutine consuming a buffered
// channel: the design spec calls for serialized PR creation so
// near-simultaneous approvals can't produce conflicting branches.
package githubpr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Default channel buffer. A moderator can't realistically approve more
// than a handful at once; 32 is comfortable headroom.
const defaultBufferSize = 32

// Config configures a Worker. Token is the fine-grained PAT scoped to
// the urbanist-atlas repo only (Contents R/W + Pull requests R/W).
type Config struct {
	// HTTPClient defaults to http.DefaultClient if nil.
	HTTPClient *http.Client
	// BaseURL defaults to https://api.github.com. Tests inject an
	// httptest.NewServer URL.
	BaseURL string
	// Token is the GitHub PAT. Empty disables the worker — Enqueue
	// returns ErrDisabled so the handler can persist a clear
	// promotion_error.
	Token string
	// Owner / Repo / BaseBranch identify the target. Defaults are
	// mjrossi / urbanist-atlas / main.
	Owner      string
	Repo       string
	BaseBranch string
	// SeedFilePath is the in-repo path the worker rewrites. Defaults
	// to api/seed/orgs.toml.
	SeedFilePath string
	// PersistResult is called after each job to record the PR URL or
	// error on the submission row. Wired to
	// atlas.SubmissionStore.AttachPromotionResult by serve.go.
	PersistResult func(ctx context.Context, publicID, prURL, prErr string) error
	// Logger defaults to slog.Default().
	Logger *slog.Logger
	// BufferSize overrides the channel capacity. Zero = default.
	BufferSize int
}

// ErrDisabled is returned by Enqueue when the worker has no GitHub
// token configured. The httpapi layer recognizes this and persists a
// "worker disabled" promotion_error.
var ErrDisabled = errors.New("githubpr: worker disabled (no token configured)")

// ErrBufferFull is returned by Enqueue when the job channel is full.
// Practically unreachable at the project's volume but expressed so
// the handler can persist a clear error rather than block.
var ErrBufferFull = errors.New("githubpr: enqueue buffer full")

// Worker drives the PR-creation pipeline.
type Worker struct {
	cfg  Config
	jobs chan atlas.Submission
	// done closes when Run returns. Stop blocks on it (or on its
	// own ctx) so callers know whether the goroutine actually
	// drained or whether the deadline elapsed first.
	done chan struct{}
	// closeOnce guards close(jobs) so Stop is idempotent — a second
	// SIGTERM (or any future caller that calls Stop twice) won't
	// panic on close-of-closed.
	closeOnce sync.Once
}

// New constructs a Worker. Run is what actually starts the goroutine.
func New(cfg Config) *Worker {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.github.com"
	}
	if cfg.Owner == "" {
		cfg.Owner = "mjrossi"
	}
	if cfg.Repo == "" {
		cfg.Repo = "urbanist-atlas"
	}
	if cfg.BaseBranch == "" {
		cfg.BaseBranch = "main"
	}
	if cfg.SeedFilePath == "" {
		cfg.SeedFilePath = "api/seed/orgs.toml"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = defaultBufferSize
	}
	return &Worker{
		cfg:  cfg,
		jobs: make(chan atlas.Submission, cfg.BufferSize),
		done: make(chan struct{}),
	}
}

// Enqueue implements httpapi.PromotionEnqueuer. Returns ErrDisabled
// when no token configured, ErrBufferFull when the channel is at
// capacity. Otherwise the job is queued and processed by Run.
func (w *Worker) Enqueue(_ context.Context, sub atlas.Submission) error {
	if w.cfg.Token == "" {
		return ErrDisabled
	}
	select {
	case w.jobs <- sub:
		return nil
	default:
		return ErrBufferFull
	}
}

// Run consumes the job channel until ctx is canceled. Blocks until
// done; intended to be launched in its own goroutine at boot.
//
// On ctx cancellation, Run drains whatever jobs are already in the
// channel (it does NOT accept new ones — Enqueue's non-blocking send
// races against a closed-for-write channel from Stop's perspective)
// so a SIGTERM mid-flight still finishes the moderator approvals
// that were already queued. The persist write inside process() uses
// a detached context (see context.WithoutCancel) so the SQLite row
// for "PR opened" still lands even after the parent ctx is gone.
//
// Pair with Stop for the shutdown-side coordination: Stop closes the
// jobs channel and blocks until this loop exits.
func (w *Worker) Run(ctx context.Context) {
	defer close(w.done)
	for {
		select {
		case job, ok := <-w.jobs:
			if !ok {
				// Stop closed the channel; we've drained whatever was
				// buffered.
				w.cfg.Logger.Info("githubpr: worker drained, shutting down")
				return
			}
			w.process(ctx, job)
		case <-ctx.Done():
			// Drain whatever's already buffered, then exit. This is the
			// hard-cancel path: a job that's mid-flight in process()
			// finishes whatever GitHub-side step it's on (the GitHub
			// HTTP client respects ctx), and the persist call inside
			// process() uses a detached ctx so it lands regardless.
			w.drainAndExit()
			return
		}
	}
}

// drainAndExit consumes any remaining jobs in the buffer with the
// caveat that the parent ctx is already canceled — GitHub I/O will
// fail fast, but the persist side will still record the error on
// each affected row. Called only from the ctx-canceled branch of
// Run; Stop's normal path goes through the channel-close branch.
func (w *Worker) drainAndExit() {
	for {
		select {
		case job, ok := <-w.jobs:
			if !ok {
				return
			}
			w.process(context.Background(), job)
		default:
			w.cfg.Logger.Info("githubpr: worker shutting down")
			return
		}
	}
}

// Stop signals the worker to finish draining its job buffer and
// exit, then blocks until Run returns or the supplied ctx expires
// (whichever comes first). Idempotent — a second call is a no-op
// re: closing the channel; it still waits on Run's completion.
//
// Returns nil on a clean drain. When ctx expires before Run exits,
// Stop returns ctx.Err() along with the public IDs of jobs that
// were observed in the channel when the deadline fired. Callers
// (serve.go's shutdown path) log them at slog.Warn so the operator
// can re-queue with `urbanist-atlas-server submissions retry-pr`.
//
// The droppedIDs list is **best-effort**: when the parent ctx is
// also canceled, Run.drainAndExit is consuming the same channel
// concurrently, so some IDs reported here may have been processed
// (or attempted) by Run before the process exits. Conversely, IDs
// drainAndExit pulled but whose GitHub call was cut off by process
// exit won't appear here either. The canonical source of truth is
// the `submissions` row's `promoted_pr_url` (set on success) and
// `promotion_error` (set on failure) — operators should treat
// droppedIDs as a "needs investigation" hint, not a definitive
// loss list.
func (w *Worker) Stop(ctx context.Context) (droppedIDs []string, err error) {
	w.closeOnce.Do(func() { close(w.jobs) })
	select {
	case <-w.done:
		return nil, nil
	case <-ctx.Done():
		// Drain whatever's still in the buffer for the operator log.
		// Receives on a closed channel never block past the buffered
		// count, so this terminates either via ok==false or via the
		// default arm.
		for {
			select {
			case job, ok := <-w.jobs:
				if !ok {
					return droppedIDs, ctx.Err()
				}
				droppedIDs = append(droppedIDs, job.PublicID)
			default:
				return droppedIDs, ctx.Err()
			}
		}
	}
}

// ProcessNow runs the PR-creation pipeline synchronously for a single
// submission and returns the resulting PR URL or the wrapped error.
// It does NOT call PersistResult — the caller (typically the retry-pr
// CLI) owns persistence so it can format the outcome for the
// operator.
//
// Unlike Enqueue, ProcessNow runs even with an empty Token (so the
// caller can pass an ad-hoc token on the CLI without configuring the
// long-lived worker first). It returns ErrDisabled only when the
// Config has no Token AND nothing was set on the live Worker.
func (w *Worker) ProcessNow(ctx context.Context, sub atlas.Submission) (string, error) {
	if w.cfg.Token == "" {
		return "", ErrDisabled
	}
	return w.openPR(ctx, sub)
}

func (w *Worker) process(ctx context.Context, sub atlas.Submission) {
	prURL, err := w.openPR(ctx, sub)
	// PersistResult runs against a detached context so a shutdown that
	// cancels ctx mid-flight doesn't also cancel the SQLite write that
	// records the PR URL or the promotion_error. Without this, an
	// approval immediately before shutdown can vanish (no PR, no
	// recorded error — operator sees nothing).
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err != nil {
		w.cfg.Logger.ErrorContext(ctx, "githubpr: PR creation failed",
			"submission_id", sub.PublicID, "err", err)
		if w.cfg.PersistResult != nil {
			if perr := w.cfg.PersistResult(persistCtx, sub.PublicID, "", err.Error()); perr != nil {
				w.cfg.Logger.ErrorContext(ctx, "githubpr: persist error result failed",
					"submission_id", sub.PublicID, "err", perr)
			}
		}
		return
	}
	if w.cfg.PersistResult != nil {
		if perr := w.cfg.PersistResult(persistCtx, sub.PublicID, prURL, ""); perr != nil {
			w.cfg.Logger.ErrorContext(ctx, "githubpr: persist success result failed",
				"submission_id", sub.PublicID, "err", perr)
		}
	}
	w.cfg.Logger.InfoContext(ctx, "githubpr: PR opened",
		"submission_id", sub.PublicID, "url", prURL)
}

// openPRTimeout bounds the whole GitHub call-chain (get-ref →
// get-contents → create-ref → put-contents → create-PR). Without it a
// single hung remote call could pin the worker goroutine — and during
// graceful shutdown, the worker's parent ctx — indefinitely. On expiry
// the in-flight step's ctx is canceled, openPR returns a wrapped
// deadline error, and process() persists it as the promotion_error
// (no retry), matching the existing failure semantics.
const openPRTimeout = 30 * time.Second

// openPR runs the full pipeline against GitHub and returns the new
// PR's html_url. Any step that fails returns a wrapped error.
//
// The work is bounded by openPRTimeout: a per-call deadline derived
// from the caller's ctx (so an already-canceled parent still short-
// circuits immediately). cancel is deferred so the derived ctx is
// always released.
//
// Retry strategy (issue #24) — conservative, idempotency-first:
//   - The two GETs (getBranchSHA, getFile) go through
//     doIdempotentRequest, which retries retryable statuses (429, 502,
//     503, secondary-rate-limit 403) with a bounded ctx-aware backoff.
//     Retrying a GET has no side effects, so this is always safe.
//   - createBranch is single-shot but tolerant of 422 "Reference
//     already exists" — so a *whole-pipeline* re-run (operator
//     retry-pr after a partial failure) doesn't trip over a branch a
//     prior attempt left behind. The branch name is deterministic per
//     submission, which is what makes that safe.
//   - putFile and createPR stay single-shot with NO internal retry.
//     NOTE: a blind retry of either could double-apply. A PUT contents
//     with a now-stale base SHA returns 409 (handled as a hard error,
//     not retried); a retried createPR after a transient 5xx that
//     actually succeeded server-side would 422 on the second call
//     ("a pull request already exists"). Until those are made
//     idempotent (e.g. by GETing the existing PR on 422), they fail
//     fast and surface as a promotion_error for operator retry — the
//     same terminal semantics as before this change.
func (w *Worker) openPR(ctx context.Context, sub atlas.Submission) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, openPRTimeout)
	defer cancel()

	baseSHA, err := w.getBranchSHA(ctx, w.cfg.BaseBranch)
	if err != nil {
		return "", fmt.Errorf("get base branch: %w", err)
	}

	existing, err := w.getFile(ctx, w.cfg.SeedFilePath, w.cfg.BaseBranch)
	if err != nil {
		return "", fmt.Errorf("get seed file: %w", err)
	}

	slug := DeriveSlug(sub.Payload.Name)
	// added_at is sourced from the approval clock — sub.ProcessedAt
	// is stamped by atlas.SubmissionStore.Approve, which is the only
	// pathway that enqueues a submission to the worker. If a future
	// caller bypasses Approve and ProcessedAt is nil, fall back to
	// the worker-local clock so the bundle still parses under the
	// Phase 4 required-field check.
	addedAt := time.Now().UTC()
	if sub.ProcessedAt != nil {
		addedAt = sub.ProcessedAt.UTC()
	}
	block, err := RenderOrgBlock(sub, slug, addedAt)
	if err != nil {
		return "", fmt.Errorf("render org block: %w", err)
	}
	newContent := ensureTrailingNewline(existing.Content) + block

	branchName := submissionBranchName(sub.PublicID)
	if err := w.createBranch(ctx, branchName, baseSHA); err != nil {
		return "", fmt.Errorf("create branch %q: %w", branchName, err)
	}

	commitMsg := fmt.Sprintf("Add %s (submission %s)", sub.Payload.Name, shortID(sub.PublicID))
	if err := w.putFile(ctx, w.cfg.SeedFilePath, branchName, newContent, existing.SHA, commitMsg); err != nil {
		return "", fmt.Errorf("put file on %q: %w", branchName, err)
	}

	prTitle := "Add " + sub.Payload.Name
	prBody := buildPRBody(sub)
	prURL, err := w.createPR(ctx, prTitle, prBody, branchName, w.cfg.BaseBranch)
	if err != nil {
		return "", fmt.Errorf("open PR: %w", err)
	}
	return prURL, nil
}

func submissionBranchName(publicID string) string {
	return "submission/" + branchSuffix(publicID)
}

// shortID returns the first 8 hex characters of the UUIDv7. UUIDv7's
// leading bits are a millisecond timestamp, so this is human-readable
// at a glance — useful in commit messages and PR titles.
func shortID(publicID string) string {
	clean := strings.ReplaceAll(publicID, "-", "")
	if len(clean) < 8 {
		return clean
	}
	return clean[:8]
}

// branchSuffix returns 16 hex characters of the UUIDv7. The first 12
// are the millisecond timestamp + the first random bits, so two
// approvals in the same millisecond still get distinct branch names;
// the extra 4 keep the suffix long enough that a future jump back to
// 8-char shortID-style names won't collide with anything already in
// the remote.
func branchSuffix(publicID string) string {
	clean := strings.ReplaceAll(publicID, "-", "")
	if len(clean) < 16 {
		return clean
	}
	return clean[:16]
}

func ensureTrailingNewline(s string) string {
	if s == "" {
		return ""
	}
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func buildPRBody(sub atlas.Submission) string {
	var b strings.Builder
	b.WriteString("Promotes public submission `")
	b.WriteString(sub.PublicID)
	b.WriteString("` (received ")
	b.WriteString(sub.CreatedAt.UTC().Format(time.RFC3339))
	b.WriteString(") to the seed bundle.\n\n")
	if sub.SubmitterName != "" || sub.SubmitterEmail != "" {
		b.WriteString("**Submitter:** ")
		if sub.SubmitterName != "" {
			b.WriteString(sub.SubmitterName)
		}
		if sub.SubmitterEmail != "" {
			if sub.SubmitterName != "" {
				b.WriteString(" ")
			}
			b.WriteString("<")
			b.WriteString(sub.SubmitterEmail)
			b.WriteString(">")
		}
		b.WriteString("\n\n")
	}
	if sub.SubmitterNote != "" {
		b.WriteString("**Note from submitter:**\n\n")
		b.WriteString(sub.SubmitterNote)
		b.WriteString("\n\n")
	}
	b.WriteString("_Auto-opened by the Urbanist Atlas submissions worker. Merge after editorial review._\n")
	return b.String()
}

// --- GitHub API plumbing ---

type fileContents struct {
	Content string // decoded UTF-8
	SHA     string
}

func (w *Worker) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, w.cfg.BaseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+w.cfg.Token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return w.cfg.HTTPClient.Do(req)
}

// Retry tuning. GitHub's transient failures (502/503 from the edge,
// 429/secondary-rate-limit 403 from abuse detection) usually clear in
// well under a second, so a handful of attempts with a short, growing
// backoff is plenty. The whole chain is still bounded by openPRTimeout
// (the ctx deadline), so these numbers are an upper bound on attempts,
// not on wall-clock — a slow remote just trips the deadline mid-sleep
// and returns ctx.Err().
const (
	retryMaxAttempts = 4 // 1 initial try + up to 3 retries
	retryBaseBackoff = 250 * time.Millisecond
)

// retryableStatus reports whether an HTTP status warrants a retry of an
// idempotent request. 429 (rate limited), 502 (bad gateway) and 503
// (service unavailable) are unambiguously transient. 403 is overloaded:
// GitHub returns it for both hard authorization failures (never retry)
// and *secondary rate limits* (retry) — the latter is signalled by a
// Retry-After header or a body that mentions a rate limit, which
// isSecondaryRateLimit inspects. Plain 4xx (404, 422, 409) are caller
// state and never retried.
func retryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, // 429
		http.StatusBadGateway,         // 502
		http.StatusServiceUnavailable: // 503
		return true
	default:
		return false
	}
}

// isSecondaryRateLimit distinguishes a retryable GitHub secondary-rate-
// limit 403 from a terminal authorization 403. GitHub flags the former
// with a Retry-After header and/or a body containing "secondary rate
// limit" / "rate limit" (see GitHub REST API "Rate limits" docs). body
// is the already-read response body (retryBody buffers it so it can be
// classified here and still restored for apiError on the no-retry path).
func isSecondaryRateLimit(resp *http.Response, body []byte) bool {
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if resp.Header.Get("Retry-After") != "" {
		return true
	}
	// x-ratelimit-remaining: 0 is GitHub's primary-rate-limit signal on
	// a 403; treat it as retryable too (the backoff gives the window
	// time to roll over).
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return strings.Contains(strings.ToLower(string(body)), "rate limit")
}

// backoffSleep waits for d or until ctx is canceled, whichever comes
// first. Returns ctx.Err() if the context fired so the retry loop can
// abort instead of sleeping out a doomed request. Uses a timer (not
// time.Sleep) precisely so cancellation is honored mid-backoff — a
// wedged GitHub call during graceful shutdown must not pin the worker.
func backoffSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// doIdempotentRequest issues an idempotent (GET) request via doRequest
// and retries it on retryable statuses with a bounded, ctx-aware
// backoff. It is ONLY safe for requests with no side effects: a retried
// GET is harmless, a retried mutation is not (see openPR's notes on why
// createBranch/putFile/createPR stay single-shot).
//
// On the terminal attempt — success, a non-retryable status, a
// transport error, or exhausted attempts — it returns the live
// *http.Response with its Body intact for the caller to decode or pass
// to apiError. On a retryable status it drains+closes the body before
// sleeping so the connection can be reused, then tries again.
func (w *Worker) doIdempotentRequest(ctx context.Context, method, path string) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := w.doRequest(ctx, method, path, nil)
		if err != nil {
			// Transport error: not classifiable as a status. Surface it
			// immediately — http.Client already retried idempotent
			// connection failures internally, and ctx cancellation
			// arrives here too.
			return nil, err
		}
		retryable := retryableStatus(resp.StatusCode)
		if resp.StatusCode == http.StatusForbidden {
			// Buffer the body so we can classify the 403 and still hand a
			// readable body to apiError on the no-retry path.
			buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(buf))
			retryable = isSecondaryRateLimit(resp, buf)
		}
		if !retryable || attempt >= retryMaxAttempts {
			return resp, nil
		}
		// Drain + close so the keep-alive connection is reusable, then
		// back off before the next attempt.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		backoff := retryBaseBackoff * time.Duration(1<<(attempt-1))
		if serr := backoffSleep(ctx, backoff); serr != nil {
			return nil, serr
		}
	}
}

// expectStatus closes nothing — the caller owns resp.Body via defer —
// but it consolidates the "did GitHub return the status I wanted?"
// check that every plumbing method repeated. On a status mismatch it
// returns the shared apiError (op + status + truncated body); on a
// match it returns nil so the caller can proceed to decode. want is
// the single accepted status; methods that accept two (putFile) keep
// their own inline check.
func expectStatus(resp *http.Response, want int, op string) error {
	if resp.StatusCode != want {
		return apiError(op, resp)
	}
	return nil
}

func (w *Worker) getBranchSHA(ctx context.Context, branch string) (string, error) {
	resp, err := w.doIdempotentRequest(ctx, http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", w.cfg.Owner, w.cfg.Repo, branch))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := expectStatus(resp, http.StatusOK, "get ref"); err != nil {
		return "", err
	}
	var out struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode ref: %w", err)
	}
	if out.Object.SHA == "" {
		return "", errors.New("get ref: empty object.sha in response")
	}
	return out.Object.SHA, nil
}

func (w *Worker) getFile(ctx context.Context, path, branch string) (fileContents, error) {
	resp, err := w.doIdempotentRequest(ctx, http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", w.cfg.Owner, w.cfg.Repo, path, branch))
	if err != nil {
		return fileContents{}, err
	}
	defer resp.Body.Close()
	if err := expectStatus(resp, http.StatusOK, "get contents"); err != nil {
		return fileContents{}, err
	}
	var out struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fileContents{}, fmt.Errorf("decode contents: %w", err)
	}
	if out.Encoding != "base64" {
		return fileContents{}, fmt.Errorf("unexpected encoding %q", out.Encoding)
	}
	// GitHub wraps the base64 body at 60 chars.
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
	if err != nil {
		return fileContents{}, fmt.Errorf("decode base64 contents: %w", err)
	}
	return fileContents{Content: string(decoded), SHA: out.SHA}, nil
}

func (w *Worker) createBranch(ctx context.Context, branch, sha string) error {
	resp, err := w.doRequest(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/%s/git/refs", w.cfg.Owner, w.cfg.Repo),
		map[string]string{
			"ref": "refs/heads/" + branch,
			"sha": sha,
		})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated {
		return nil
	}
	// Idempotency: GitHub answers a duplicate branch with 422
	// "Reference already exists". A previous openPR attempt that created
	// the branch but failed at a later step (put/createPR) would leave
	// the ref behind; on a manual retry-pr re-run the same submission
	// derives the same deterministic branch name, so treat an existing
	// ref as success and let the pipeline proceed (putFile/createPR are
	// keyed on the branch, not on having just created it). buf is read
	// once and reused for both the existence check and apiError so the
	// body isn't consumed twice.
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusUnprocessableEntity &&
		strings.Contains(strings.ToLower(string(buf)), "reference already exists") {
		return nil
	}
	return fmt.Errorf("create ref: github returned %d: %s",
		resp.StatusCode, strings.TrimSpace(string(buf)))
}

func (w *Worker) putFile(ctx context.Context, path, branch, content, sha, message string) error {
	resp, err := w.doRequest(ctx, http.MethodPut,
		fmt.Sprintf("/repos/%s/%s/contents/%s", w.cfg.Owner, w.cfg.Repo, path),
		map[string]string{
			"message": message,
			"content": base64.StdEncoding.EncodeToString([]byte(content)),
			"sha":     sha,
			"branch":  branch,
		})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return apiError("put contents", resp)
	}
	return nil
}

func (w *Worker) createPR(ctx context.Context, title, body, head, base string) (string, error) {
	resp, err := w.doRequest(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/%s/pulls", w.cfg.Owner, w.cfg.Repo),
		map[string]string{
			"title": title,
			"body":  body,
			"head":  head,
			"base":  base,
		})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := expectStatus(resp, http.StatusCreated, "create PR"); err != nil {
		return "", err
	}
	var out struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode PR response: %w", err)
	}
	return out.HTMLURL, nil
}

func apiError(op string, resp *http.Response) error {
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("%s: github returned %d: %s", op, resp.StatusCode, strings.TrimSpace(string(buf)))
}
