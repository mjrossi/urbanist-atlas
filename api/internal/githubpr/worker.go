// Package githubpr is the asynchronous worker that opens a GitHub pull
// request appending an approved submission to api/seed/orgs.toml. The PR
// is the editorial-review surface; once merged, the next deploy ships the
// org.
//
// One goroutine consumes a buffered channel, so PRs are created one at a
// time and near-simultaneous approvals can't produce conflicting
// branches.
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
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// defaultBufferSize is far more approvals than a moderator makes at once.
const defaultBufferSize = 32

// Config configures a Worker. Token is a fine-grained PAT scoped to the
// repo (Contents and Pull requests read/write).
type Config struct {
	// HTTPClient defaults to http.DefaultClient if nil.
	HTTPClient *http.Client
	// BaseURL defaults to https://api.github.com.
	BaseURL string
	// Token is the GitHub PAT. Empty disables the worker: Enqueue
	// returns atlas.ErrPromotionDisabled.
	Token string
	// Owner / Repo / BaseBranch default to mjrossi / urbanist-atlas / main.
	Owner      string
	Repo       string
	BaseBranch string
	// SeedFilePath defaults to api/seed/orgs.toml.
	SeedFilePath string
	// PersistResult records each job's PR URL or error on the submission
	// row.
	PersistResult func(ctx context.Context, publicID, prURL, prErr string) error
	// Logger defaults to slog.Default().
	Logger *slog.Logger
	// BufferSize overrides the channel capacity; zero means the default.
	BufferSize int
}

// ErrBufferFull is returned by Enqueue when the job channel is full,
// rather than blocking the caller.
var ErrBufferFull = errors.New("githubpr: enqueue buffer full")

// ErrStopped is returned by Enqueue after Stop has begun.
var ErrStopped = errors.New("githubpr: worker stopped")

// Worker drives the PR-creation pipeline.
type Worker struct {
	cfg  Config
	jobs chan atlas.Submission
	// done closes when Run returns, so Stop can tell a drain from a
	// deadline.
	done chan struct{}
	// mu guards stopped. Enqueue holds the read lock across its send so
	// Stop can't close jobs between the check and the send.
	mu      sync.RWMutex
	stopped bool
	// inflight is the public ID of the job process() is running, so a
	// Stop that times out can report it as dropped.
	inflight atomic.Pointer[string]
}

// New constructs a Worker; Run starts it.
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

// Enqueue queues sub for Run. It returns atlas.ErrPromotionDisabled
// without a token, ErrStopped once Stop has begun, and ErrBufferFull when
// the channel is at capacity.
func (w *Worker) Enqueue(_ context.Context, sub atlas.Submission) error {
	if w.cfg.Token == "" {
		return atlas.ErrPromotionDisabled
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.stopped {
		return ErrStopped
	}
	select {
	case w.jobs <- sub:
		return nil
	default:
		return ErrBufferFull
	}
}

// Run consumes the job channel until Stop closes it, then returns. It
// blocks, so launch it in its own goroutine at boot.
//
// ctx must outlive HTTP request handling, so don't pass the signal
// context: approvals keep arriving while the server drains, and a job
// enqueued after Run returned would sit in the buffer unread, with no
// PR and no promotion_error. Canceling ctx is a hard abort: in-flight
// GitHub calls are cut off and buffered jobs are attempted under
// shutdownDrainTimeout, each outcome still persisted.
func (w *Worker) Run(ctx context.Context) {
	defer close(w.done)
	for {
		select {
		case job, ok := <-w.jobs:
			if !ok {
				w.cfg.Logger.Info("githubpr: worker drained, shutting down")
				return
			}
			w.processTracked(ctx, job)
		case <-ctx.Done():
			w.drainAndExit()
			return
		}
	}
}

func (w *Worker) processTracked(ctx context.Context, job atlas.Submission) {
	w.inflight.Store(&job.PublicID)
	defer w.inflight.Store(nil)
	w.process(ctx, job)
}

// shutdownDrainTimeout bounds each job in the hard-cancel drain. Those
// jobs run on a detached context so their result still persists, and
// without this they would get openPR's full 30s each. It is a var only so
// a test can shrink it.
var shutdownDrainTimeout = 5 * time.Second

// drainAndExit runs the jobs left in the buffer after ctx is canceled,
// each on a fresh context bounded by shutdownDrainTimeout, so every row
// still records its outcome.
func (w *Worker) drainAndExit() {
	for {
		select {
		case job, ok := <-w.jobs:
			if !ok {
				return
			}
			drainCtx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
			w.processTracked(drainCtx, job)
			cancel()
		default:
			w.cfg.Logger.Info("githubpr: worker shutting down")
			return
		}
	}
}

// Stop makes Enqueue refuse new jobs, lets Run finish the buffered
// ones, and blocks until Run returns or ctx expires. Idempotent.
//
// Returns nil on a clean drain. When ctx expires first, Stop returns
// ctx.Err() and the public IDs of the in-flight job and of jobs still
// buffered, for the operator to re-queue with `urbanist-atlas-server
// submissions retry-pr`. The list is best-effort, because Run keeps
// consuming the buffer concurrently. The submission row's
// promotion_pr_url / promotion_error is the source of truth.
func (w *Worker) Stop(ctx context.Context) (droppedIDs []string, err error) {
	w.mu.Lock()
	if !w.stopped {
		w.stopped = true
		close(w.jobs)
	}
	w.mu.Unlock()
	select {
	case <-w.done:
		return nil, nil
	case <-ctx.Done():
		if id := w.inflight.Load(); id != nil {
			droppedIDs = append(droppedIDs, *id)
		}
		// Receives on a closed channel stop at the buffered count, so this
		// terminates.
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
// It does not call PersistResult: the caller owns persistence. Like
// Enqueue, it returns atlas.ErrPromotionDisabled when
// Config.Token is empty.
func (w *Worker) ProcessNow(ctx context.Context, sub atlas.Submission) (string, error) {
	if w.cfg.Token == "" {
		return "", atlas.ErrPromotionDisabled
	}
	return w.openPR(ctx, sub)
}

func (w *Worker) process(ctx context.Context, sub atlas.Submission) {
	prURL, err := w.openPR(ctx, sub)
	// Persist on a detached context: a shutdown that cancels ctx must
	// not also cancel the write recording the PR URL or error.
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

// openPRTimeout bounds the whole GitHub call chain, so one hung remote
// call can't pin the worker, or shutdown, indefinitely. On expiry the
// error is persisted like any other failure, with no retry.
const openPRTimeout = 30 * time.Second

// openPR runs the GitHub pipeline and returns the new PR's html_url.
// Only the GETs retry (doIdempotentRequest), since a blind retry of a
// mutation could double-apply. A branch left by an earlier attempt is
// force-reset to the base, so the result equals a fresh run, and a 422
// from createPR resolves to the PR already open on the branch.
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
	// ProcessedAt is the approval time; fall back to now so added_at is
	// never empty.
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

	commitMsg := fmt.Sprintf("Add %s (submission %s)", codeSpan(sub.Payload.Name), shortID(sub.PublicID))
	if err := w.putFile(ctx, w.cfg.SeedFilePath, branchName, newContent, existing.SHA, commitMsg); err != nil {
		return "", fmt.Errorf("put file on %q: %w", branchName, err)
	}

	prTitle := "Add " + codeSpan(sub.Payload.Name)
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

// shortID returns the last 8 hex characters of the UUIDv7. The leading
// characters are its millisecond timestamp, so a prefix would collide
// for submissions made about a minute apart; the tail is random.
func shortID(publicID string) string {
	clean := strings.ReplaceAll(publicID, "-", "")
	if len(clean) < 8 {
		return clean
	}
	return clean[len(clean)-8:]
}

// branchSuffix returns the first 16 hex characters of the UUIDv7: the
// millisecond timestamp, the version nibble and the 12-bit rand_a, which
// google/uuid fills with a per-process sequence, so same-millisecond
// approvals still get distinct branches.
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

// buildPRBody carries no submitter name, email or note: the repo is
// public, and the submit form promises the email is only for editor
// follow-up.
func buildPRBody(sub atlas.Submission) string {
	var b strings.Builder
	b.WriteString("Promotes public submission ")
	b.WriteString(codeSpan(sub.PublicID))
	b.WriteString(" (received ")
	b.WriteString(sub.CreatedAt.UTC().Format(time.RFC3339))
	b.WriteString(") to the seed bundle.\n\n")
	b.WriteString("Regions: ")
	if len(sub.Payload.RegionSlugs) == 0 {
		b.WriteString("none")
	}
	for i, slug := range sub.Payload.RegionSlugs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(codeSpan(slug))
	}
	b.WriteString("\n\n")
	b.WriteString("Submitter contact details and notes are private. Editors can read them through the admin API, ")
	b.WriteString("`GET /api/v1/admin/submissions`, by the submission ID above.\n\n")
	b.WriteString("_Auto-opened by the Urbanist Atlas submissions worker. Merge after editorial review._\n")
	return b.String()
}

// codeSpan wraps submitter-controlled text in a Markdown code span, so
// GitHub renders it literally: no @mention notifications, links or
// formatting. Backticks are dropped so the text can't close the span,
// and line breaks are flattened so it can't start a block.
func codeSpan(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.Join(strings.Fields(s), " ")
	return "`" + s + "`"
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

// Retry tuning. GitHub's transient failures usually clear in well under
// a second. openPRTimeout still bounds the wall clock: a slow remote
// trips the deadline mid-sleep.
const (
	retryMaxAttempts = 4 // 1 initial try + up to 3 retries
	retryBaseBackoff = 250 * time.Millisecond
)

// retryableStatus reports whether a status is plainly transient (429,
// 502, 503). A 403 can be a hard authorization failure or a secondary
// rate limit, so doIdempotentRequest classifies it with
// isSecondaryRateLimit instead.
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

// isSecondaryRateLimit reports whether a 403 is a retryable rate limit
// rather than an authorization failure. GitHub signals one with
// Retry-After, X-RateLimit-Remaining: 0, or a body mentioning a rate
// limit. body is the already-buffered response body.
func isSecondaryRateLimit(resp *http.Response, body []byte) bool {
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if resp.Header.Get("Retry-After") != "" {
		return true
	}
	// A primary limit's reset can be an hour out, so it is not consulted:
	// retrying costs a few hundred ms and recovers a short secondary limit
	// that set no Retry-After.
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return strings.Contains(strings.ToLower(string(body)), "rate limit")
}

// backoffSleep waits for d, or returns ctx.Err() as soon as ctx is done.
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

// doIdempotentRequest sends a side-effect-free request, retrying
// retryable statuses with a bounded backoff. The final response comes
// back with its Body intact for the caller to decode or pass to
// apiError.
func (w *Worker) doIdempotentRequest(ctx context.Context, method, path string) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := w.doRequest(ctx, method, path, nil)
		if err != nil {
			// http.Client already retried idempotent connection failures, and
			// ctx cancellation lands here too.
			return nil, err
		}
		retryable := retryableStatus(resp.StatusCode)
		if resp.StatusCode == http.StatusForbidden {
			// Buffer the body so apiError can still read it.
			buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(buf))
			retryable = isSecondaryRateLimit(resp, buf)
		}
		if !retryable || attempt >= retryMaxAttempts {
			return resp, nil
		}
		// Drain so the keep-alive connection is reusable.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		backoff := retryBaseBackoff * time.Duration(1<<(attempt-1))
		if serr := backoffSleep(ctx, backoff); serr != nil {
			return nil, serr
		}
	}
}

// expectStatus returns apiError unless resp has status want. It leaves
// resp.Body to the caller.
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
	// 422 "Reference already exists" means a prior attempt left this
	// submission's branch behind, possibly with its own commit or on an
	// older base. Force-resetting it to sha keeps the branch's orgs.toml
	// blob equal to the base's, which is the SHA putFile sends. If that
	// attempt also opened a PR, createPR resolves its duplicate 422 to it.
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusUnprocessableEntity &&
		strings.Contains(strings.ToLower(string(buf)), "reference already exists") {
		return w.resetBranch(ctx, branch, sha)
	}
	return fmt.Errorf("create ref: github returned %d: %s",
		resp.StatusCode, strings.TrimSpace(string(buf)))
}

func (w *Worker) resetBranch(ctx context.Context, branch, sha string) error {
	resp, err := w.doRequest(ctx, http.MethodPatch,
		fmt.Sprintf("/repos/%s/%s/git/refs/heads/%s", w.cfg.Owner, w.cfg.Repo, branch),
		map[string]any{
			"sha":   sha,
			"force": true,
		})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return expectStatus(resp, http.StatusOK, "reset ref")
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
	if resp.StatusCode == http.StatusUnprocessableEntity {
		// GitHub answers 422 when head already has an open PR into base,
		// and also for other validation failures. Only an open PR found
		// by the lookup turns the 422 into success.
		createErr := apiError("create PR", resp)
		prURL, err := w.findOpenPR(ctx, head, base)
		switch {
		case err != nil:
			return "", fmt.Errorf("%w (existing PR lookup: %w)", createErr, err)
		case prURL == "":
			return "", createErr
		default:
			return prURL, nil
		}
	}
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

// findOpenPR returns the html_url of the open PR from head into base, or
// "" when there is none.
func (w *Worker) findOpenPR(ctx context.Context, head, base string) (string, error) {
	q := url.Values{
		"head":  {w.cfg.Owner + ":" + head},
		"base":  {base},
		"state": {"open"},
	}
	resp, err := w.doIdempotentRequest(ctx, http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/pulls?%s", w.cfg.Owner, w.cfg.Repo, q.Encode()))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := expectStatus(resp, http.StatusOK, "list PRs"); err != nil {
		return "", err
	}
	var out []struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode PR list: %w", err)
	}
	if len(out) == 0 {
		return "", nil
	}
	return out[0].HTMLURL, nil
}

func apiError(op string, resp *http.Response) error {
	// The status is the signal and the capped body is context; a read
	// error is marked so it can't pass for an empty body.
	buf, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	body := strings.TrimSpace(string(buf))
	if readErr != nil {
		body += fmt.Sprintf(" [body read truncated: %v]", readErr)
	}
	return fmt.Errorf("%s: github returned %d: %s", op, resp.StatusCode, body)
}
