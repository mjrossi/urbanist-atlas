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
func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.cfg.Logger.Info("githubpr: worker shutting down")
			return
		case job := <-w.jobs:
			w.process(ctx, job)
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
	if err != nil {
		w.cfg.Logger.ErrorContext(ctx, "githubpr: PR creation failed",
			"submission_id", sub.PublicID, "err", err)
		if w.cfg.PersistResult != nil {
			if perr := w.cfg.PersistResult(ctx, sub.PublicID, "", err.Error()); perr != nil {
				w.cfg.Logger.ErrorContext(ctx, "githubpr: persist error result failed",
					"submission_id", sub.PublicID, "err", perr)
			}
		}
		return
	}
	if w.cfg.PersistResult != nil {
		if perr := w.cfg.PersistResult(ctx, sub.PublicID, prURL, ""); perr != nil {
			w.cfg.Logger.ErrorContext(ctx, "githubpr: persist success result failed",
				"submission_id", sub.PublicID, "err", perr)
		}
	}
	w.cfg.Logger.InfoContext(ctx, "githubpr: PR opened",
		"submission_id", sub.PublicID, "url", prURL)
}

// openPR runs the full pipeline against GitHub and returns the new
// PR's html_url. Any step that fails returns a wrapped error.
func (w *Worker) openPR(ctx context.Context, sub atlas.Submission) (string, error) {
	baseSHA, err := w.getBranchSHA(ctx, w.cfg.BaseBranch)
	if err != nil {
		return "", fmt.Errorf("get base branch: %w", err)
	}

	existing, err := w.getFile(ctx, w.cfg.SeedFilePath, w.cfg.BaseBranch)
	if err != nil {
		return "", fmt.Errorf("get seed file: %w", err)
	}

	slug := DeriveSlug(sub.Payload.Name)
	block, err := RenderOrgBlock(sub, slug)
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

	prTitle := fmt.Sprintf("Add %s", sub.Payload.Name)
	prBody := buildPRBody(sub)
	prURL, err := w.createPR(ctx, prTitle, prBody, branchName, w.cfg.BaseBranch)
	if err != nil {
		return "", fmt.Errorf("open PR: %w", err)
	}
	return prURL, nil
}

func submissionBranchName(publicID string) string {
	return "submission/" + shortID(publicID)
}

// shortID returns the first 8 characters of the UUIDv7 (i.e. the
// most-significant time bits). UUIDv7 starts with a millisecond
// timestamp so the prefix is unique enough for branch-name use over
// the project's foreseeable submission volume.
func shortID(publicID string) string {
	clean := strings.ReplaceAll(publicID, "-", "")
	if len(clean) < 8 {
		return clean
	}
	return clean[:8]
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

func (w *Worker) getBranchSHA(ctx context.Context, branch string) (string, error) {
	resp, err := w.doRequest(ctx, http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", w.cfg.Owner, w.cfg.Repo, branch), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", apiError("get ref", resp)
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
	resp, err := w.doRequest(ctx, http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", w.cfg.Owner, w.cfg.Repo, path, branch), nil)
	if err != nil {
		return fileContents{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fileContents{}, apiError("get contents", resp)
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
	if resp.StatusCode != http.StatusCreated {
		return apiError("create ref", resp)
	}
	return nil
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
	if resp.StatusCode != http.StatusCreated {
		return "", apiError("create PR", resp)
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
