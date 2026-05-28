package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// ErrPromotionWorkerDisabled is the sentinel returned by a
// PromotionEnqueuer when no GitHub credentials are configured. The
// admin/approve handler maps it onto promotion_error so the caller
// (or the PR retry CLI) knows the row was approved but the PR was
// never queued.
var ErrPromotionWorkerDisabled = errors.New("httpapi: promotion worker disabled (no token configured)")

// PromotionEnqueuer is the seam the approve handler uses to hand an
// approved submission off to the GitHub PR worker. Implementations
// must return promptly — they should buffer the job and return nil,
// not block on GitHub I/O. ErrPromotionWorkerDisabled is a recognized
// "no worker configured" signal; any other error is treated as
// "enqueue failed, persist promotion_error".
type PromotionEnqueuer interface {
	Enqueue(ctx context.Context, sub atlas.Submission) error
}

// Maximum body size we'll read from a public submission. Way more
// than the form needs (~1 KiB typical) and small enough that a
// malicious client can't pin a worker on JSON parsing.
const submissionBodyLimit = 64 * 1024

// createSubmissionHandler answers POST /api/v1/submissions. Public
// endpoint; gated by clientSecretMiddleware upstream + rate-limited
// per-IP below.
func createSubmissionHandler(subs atlas.SubmissionStore, regions atlas.Store, limiter *ipRateLimiter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		if ok, retry := limiter.allow(clientIP(r)); !ok {
			writeRateLimited(w, r, retry, rid)
			return
		}

		var body oapi.NewSubmissionRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, submissionBodyLimit))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			detail := "request body is not valid JSON for NewSubmissionRequest"
			if errors.Is(err, io.EOF) {
				detail = "request body is empty"
			}
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Bad Request", detail, rid)
			return
		}

		payload := atlas.SubmissionPayload{
			Name:        strings.TrimSpace(body.Payload.Name),
			ShortDesc:   strings.TrimSpace(body.Payload.ShortDesc),
			WebsiteURL:  strings.TrimSpace(body.Payload.WebsiteUrl),
			Tags:        normalizeStringSlice(body.Payload.Tags),
			RegionSlugs: normalizeStringSlice(body.Payload.RegionSlugs),
		}
		if body.Payload.ContactUrl != nil {
			payload.ContactURL = strings.TrimSpace(*body.Payload.ContactUrl)
		}

		if err := seedfiles.ValidateOrgFields(payload.Name, payload.ShortDesc, payload.WebsiteURL, payload.RegionSlugs); err != nil {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Bad Request", err.Error(), rid)
			return
		}
		if err := validateRegionSlugs(r.Context(), regions, payload.RegionSlugs); err != nil {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Bad Request", err.Error(), rid)
			return
		}

		in := atlas.NewSubmissionInput{Payload: payload}
		if body.SubmitterName != nil {
			in.SubmitterName = strings.TrimSpace(*body.SubmitterName)
		}
		if body.SubmitterEmail != nil {
			in.SubmitterEmail = strings.TrimSpace(string(*body.SubmitterEmail))
		}
		if body.SubmitterNote != nil {
			in.SubmitterNote = *body.SubmitterNote
		}

		sub, err := subs.Create(r.Context(), in)
		if err != nil {
			logger.ErrorContext(r.Context(), "create submission failed", "err", err, "rid", rid)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error", "internal error", rid)
			return
		}
		writeJSON(w, http.StatusCreated, toOAPISubmission(sub))
	}
}

// listSubmissionsHandler answers GET /api/v1/admin/submissions.
// Bearer-gated. Defaults to status=pending.
func listSubmissionsHandler(subs atlas.SubmissionStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		q := atlas.ListSubmissionsQuery{Status: atlas.SubmissionPending}
		if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
			switch atlas.SubmissionStatus(raw) {
			case atlas.SubmissionPending, atlas.SubmissionApproved, atlas.SubmissionRejected:
				q.Status = atlas.SubmissionStatus(raw)
			default:
				writeProblem(w, r, http.StatusBadRequest, problemValidation, "Bad Request",
					"status must be one of pending, approved, rejected", rid)
				return
			}
		}
		rows, err := subs.List(r.Context(), q)
		if err != nil {
			logger.ErrorContext(r.Context(), "list submissions failed", "err", err, "rid", rid)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error", "internal error", rid)
			return
		}
		out := make([]oapi.Submission, 0, len(rows))
		for _, s := range rows {
			out = append(out, toOAPISubmission(s))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// approveSubmissionHandler answers POST /api/v1/admin/submissions/{id}/approve.
func approveSubmissionHandler(subs atlas.SubmissionStore, enq PromotionEnqueuer, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		publicID, ok := readSubmissionID(w, r, rid)
		if !ok {
			return
		}
		sub, err := subs.Approve(r.Context(), publicID)
		if err != nil {
			writeSubmissionStateErr(w, r, err, rid, logger, "approve submission")
			return
		}

		// Best-effort enqueue. The row is already approved; the worker
		// is free to fail without un-doing the moderator's decision.
		enqErr := enqueueOrDisabled(r.Context(), enq, sub)
		if enqErr != nil {
			attachErr := subs.AttachPromotionResult(r.Context(), sub.PublicID, "", enqErr.Error())
			if attachErr != nil {
				logger.ErrorContext(r.Context(), "attach promotion_error after enqueue failure", "err", attachErr, "rid", rid)
			}
			// Re-read so the response reflects the persisted error.
			if reread, rerr := subs.Get(r.Context(), sub.PublicID); rerr == nil {
				sub = reread
			}
		}

		writeJSON(w, http.StatusOK, toOAPISubmission(sub))
	}
}

// rejectSubmissionHandler answers POST /api/v1/admin/submissions/{id}/reject.
func rejectSubmissionHandler(subs atlas.SubmissionStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		publicID, ok := readSubmissionID(w, r, rid)
		if !ok {
			return
		}
		var body oapi.RejectSubmissionRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, submissionBodyLimit))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Bad Request",
				"request body is not valid JSON for RejectSubmissionRequest", rid)
			return
		}
		reason := strings.TrimSpace(body.Reason)
		if reason == "" {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Bad Request",
				"reason required", rid)
			return
		}
		sub, err := subs.Reject(r.Context(), publicID, reason)
		if err != nil {
			writeSubmissionStateErr(w, r, err, rid, logger, "reject submission")
			return
		}
		writeJSON(w, http.StatusOK, toOAPISubmission(sub))
	}
}

// readSubmissionID extracts and validates the {id} path param. Writes
// a 400 problem document and returns false on a malformed UUID.
func readSubmissionID(w http.ResponseWriter, r *http.Request, rid string) (string, bool) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	if _, err := uuid.Parse(raw); err != nil {
		writeProblem(w, r, http.StatusBadRequest, problemValidation, "Bad Request",
			"submission id is not a valid UUID", rid)
		return "", false
	}
	return raw, true
}

// writeSubmissionStateErr maps the well-known SubmissionStore errors
// onto their HTTP equivalents. Unknown errors are 500.
func writeSubmissionStateErr(w http.ResponseWriter, r *http.Request, err error, rid string, logger *slog.Logger, op string) {
	switch {
	case errors.Is(err, atlas.ErrSubmissionNotFound):
		writeProblem(w, r, http.StatusNotFound, problemNotFound, "Not Found",
			"no submission with that id", rid)
	case errors.Is(err, atlas.ErrSubmissionNotPending):
		writeProblem(w, r, http.StatusConflict, problemConflict, "Conflict",
			"submission has already been processed", rid)
	default:
		logger.ErrorContext(r.Context(), op+" failed", "err", err, "rid", rid)
		writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error", "internal error", rid)
	}
}

func enqueueOrDisabled(ctx context.Context, enq PromotionEnqueuer, sub atlas.Submission) error {
	if enq == nil {
		return ErrPromotionWorkerDisabled
	}
	return enq.Enqueue(ctx, sub)
}

func validateRegionSlugs(ctx context.Context, regions atlas.Store, slugs []string) error {
	if regions == nil {
		// Defensive: a missing read-side store at this point would be a
		// programmer error. Return validation failure rather than crash.
		return errors.New("region resolver unavailable")
	}
	seen := map[string]bool{}
	for _, slug := range slugs {
		if seen[slug] {
			return fmt.Errorf("region_slugs contains duplicate %q", slug)
		}
		seen[slug] = true
		if _, err := regions.ResolveRegionBySlug(ctx, slug); err != nil {
			if errors.Is(err, atlas.ErrRegionNotFound) {
				return fmt.Errorf("region_slugs contains unknown slug %q", slug)
			}
			return fmt.Errorf("region lookup failed for %q: %w", slug, err)
		}
	}
	return nil
}

func normalizeStringSlice(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func toOAPISubmission(s atlas.Submission) oapi.Submission {
	id, _ := uuid.Parse(s.PublicID)
	out := oapi.Submission{
		Id:        id,
		Status:    oapi.SubmissionStatus(s.Status),
		CreatedAt: s.CreatedAt,
		Payload:   toOAPISubmissionPayload(s.Payload),
	}
	if s.ProcessedAt != nil {
		t := *s.ProcessedAt
		out.ProcessedAt = &t
	}
	if s.SubmitterName != "" {
		n := s.SubmitterName
		out.SubmitterName = &n
	}
	if s.SubmitterEmail != "" {
		e := openapi_types.Email(s.SubmitterEmail)
		out.SubmitterEmail = &e
	}
	if s.SubmitterNote != "" {
		n := s.SubmitterNote
		out.SubmitterNote = &n
	}
	if s.PromotionPRURL != "" {
		v := s.PromotionPRURL
		out.PromotionPrUrl = &v
	}
	if s.PromotionError != "" {
		v := s.PromotionError
		out.PromotionError = &v
	}
	if s.RejectionReason != "" {
		v := s.RejectionReason
		out.RejectionReason = &v
	}
	return out
}

func toOAPISubmissionPayload(p atlas.SubmissionPayload) oapi.SubmissionPayload {
	tags := append([]string(nil), p.Tags...)
	if tags == nil {
		tags = []string{}
	}
	regions := append([]string(nil), p.RegionSlugs...)
	if regions == nil {
		regions = []string{}
	}
	out := oapi.SubmissionPayload{
		Name:        p.Name,
		ShortDesc:   p.ShortDesc,
		WebsiteUrl:  p.WebsiteURL,
		Tags:        tags,
		RegionSlugs: regions,
	}
	if p.ContactURL != "" {
		c := p.ContactURL
		out.ContactUrl = &c
	}
	return out
}
