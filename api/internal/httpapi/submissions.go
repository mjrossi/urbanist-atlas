package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
	"github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite"
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
			title := "Invalid Request Body"
			detail := "The request body is not valid JSON for NewSubmissionRequest."
			if errors.Is(err, io.EOF) {
				title = "Empty Request Body"
				detail = "The request body is empty; expected a NewSubmissionRequest JSON object."
			}
			// http.MaxBytesReader surfaces an oversize body as a
			// MaxBytesError; surface that with the 413-shape title even
			// though we keep the 400 status (oversize submissions are a
			// validation failure from the client's perspective).
			var mbErr *http.MaxBytesError
			if errors.As(err, &mbErr) {
				title = "Request Body Too Large"
				detail = fmt.Sprintf("The request body exceeds the maximum size of %d bytes.", submissionBodyLimit)
			}
			writeProblem(w, r, http.StatusBadRequest, problemValidation, title, detail, rid)
			return
		}

		// region_slugs is optional on the wire; the SPA's region field
		// is free-form text and most submissions don't carry a
		// canonical slug. nil pointer and empty slice both flow through
		// as zero-length input to ValidateSubmissionPayload.
		var rawRegionSlugs []string
		if body.Payload.RegionSlugs != nil {
			rawRegionSlugs = *body.Payload.RegionSlugs
		}
		payload := atlas.SubmissionPayload{
			Name:        strings.TrimSpace(body.Payload.Name),
			ShortDesc:   strings.TrimSpace(body.Payload.ShortDesc),
			WebsiteURL:  strings.TrimSpace(body.Payload.WebsiteUrl),
			Tags:        normalizeStringSlice(body.Payload.Tags),
			RegionSlugs: normalizeStringSlice(rawRegionSlugs),
		}
		if body.Payload.ContactUrl != nil {
			payload.ContactURL = strings.TrimSpace(*body.Payload.ContactUrl)
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

		fieldErrs := seedfiles.ValidateSubmissionPayload(
			seedfiles.SubmissionPayloadInput{
				Name:        payload.Name,
				ShortDesc:   payload.ShortDesc,
				WebsiteURL:  payload.WebsiteURL,
				ContactURL:  payload.ContactURL,
				Tags:        payload.Tags,
				RegionSlugs: payload.RegionSlugs,
			},
			seedfiles.SubmitterInput{
				Name:  in.SubmitterName,
				Email: in.SubmitterEmail,
				Note:  in.SubmitterNote,
			},
		)

		// Region-slug existence is the one check the shared validator
		// can't do (it needs the store + context). Run it only when
		// shape validation already passed for `region_slugs`, then merge
		// the result into the field-errors map so the client receives a
		// single per-field response.
		if _, alreadyBad := fieldErrs["region_slugs"]; !alreadyBad {
			if msg := checkRegionSlugsExist(r.Context(), regions, payload.RegionSlugs); msg != "" {
				if fieldErrs == nil {
					fieldErrs = map[string]string{}
				}
				fieldErrs["region_slugs"] = msg
			}
		}

		if len(fieldErrs) > 0 {
			writeProblemWithErrors(w, r, http.StatusBadRequest, problemValidation,
				"Submission Validation Failed",
				"One or more fields in the submission failed validation. See the errors map for per-field messages.",
				rid, fieldErrs)
			return
		}

		sub, err := subs.Create(r.Context(), in)
		if err != nil {
			logger.ErrorContext(r.Context(), "create submission failed", "err", err, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		writeJSON(w, http.StatusCreated, toOAPISubmission(sub))
	}
}

// listSubmissionsHandler answers GET /api/v1/admin/submissions.
// Bearer-gated. Defaults to status=pending. Returns at most ?limit=
// rows (default 50, max 200) ordered newest-first. When more rows
// exist, an `X-Next-Cursor` response header carries an opaque token
// the client passes back as `?cursor=` for the next page.
func listSubmissionsHandler(subs atlas.SubmissionStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		q := atlas.ListSubmissionsQuery{Status: atlas.SubmissionPending}
		if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
			switch atlas.SubmissionStatus(raw) {
			case atlas.SubmissionPending, atlas.SubmissionApproved, atlas.SubmissionRejected:
				q.Status = atlas.SubmissionStatus(raw)
			default:
				writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Status Filter",
					"The status query parameter must be one of pending, approved, or rejected.", rid)
				return
			}
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > maxAdminListLimit {
				writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Limit",
					fmt.Sprintf("The limit query parameter must be an integer between 1 and %d.", maxAdminListLimit), rid)
				return
			}
			q.Limit = n
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
			q.Cursor = raw
		}
		page, err := subs.ListPage(r.Context(), q)
		if err != nil {
			if errors.Is(err, sqlite.ErrInvalidCursor) {
				writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Cursor",
					"The cursor query parameter is malformed; pass the value of the previous response's X-Next-Cursor header.", rid)
				return
			}
			logger.ErrorContext(r.Context(), "list submissions failed", "err", err, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		if page.NextCursor != "" {
			w.Header().Set("X-Next-Cursor", page.NextCursor)
		}
		out := make([]oapi.Submission, 0, len(page.Items))
		for _, s := range page.Items {
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
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Request Body",
				"The request body is not valid JSON for RejectSubmissionRequest.", rid)
			return
		}
		reason := strings.TrimSpace(body.Reason)
		if reason == "" {
			writeProblemWithErrors(w, r, http.StatusBadRequest, problemValidation,
				"Submission Validation Failed",
				"A rejection reason is required.", rid,
				map[string]string{"reason": "A rejection reason is required."})
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
		writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Submission ID",
			"The submission id path parameter is not a valid UUID.", rid)
		return "", false
	}
	return raw, true
}

// writeSubmissionStateErr maps the well-known SubmissionStore errors
// onto their HTTP equivalents. Unknown errors are 500.
func writeSubmissionStateErr(w http.ResponseWriter, r *http.Request, err error, rid string, logger *slog.Logger, op string) {
	switch {
	case errors.Is(err, atlas.ErrSubmissionNotFound):
		writeProblem(w, r, http.StatusNotFound, problemNotFound, "Submission Not Found",
			"No submission matches that id.", rid)
	case errors.Is(err, atlas.ErrSubmissionNotPending):
		writeProblem(w, r, http.StatusConflict, problemConflict, "Submission Already Processed",
			"This submission has already been approved or rejected and cannot transition again.", rid)
	default:
		logger.ErrorContext(r.Context(), op+" failed", "err", err, "rid", rid)
		writeInternalProblem(w, r, rid)
	}
}

func enqueueOrDisabled(ctx context.Context, enq PromotionEnqueuer, sub atlas.Submission) error {
	if enq == nil {
		return ErrPromotionWorkerDisabled
	}
	return enq.Enqueue(ctx, sub)
}

// checkRegionSlugsExist resolves each region slug against the store
// and returns a sentence-form message naming the first offender (or
// "" when every slug resolves and no duplicates are present). The
// existence check requires a context-bound store call so it lives
// here rather than in seedfiles.ValidateSubmissionPayload.
func checkRegionSlugsExist(ctx context.Context, regions atlas.Store, slugs []string) string {
	if regions == nil {
		// Defensive: a missing read-side store at this point would be
		// a programmer error. Surface it as a validation failure rather
		// than crash.
		return "Region resolver is unavailable; cannot verify region slugs."
	}
	seen := map[string]bool{}
	for _, slug := range slugs {
		if seen[slug] {
			return fmt.Sprintf("Region slug %q appears more than once.", slug)
		}
		seen[slug] = true
		if _, err := regions.ResolveRegionBySlug(ctx, slug); err != nil {
			if errors.Is(err, atlas.ErrRegionNotFound) {
				return fmt.Sprintf("Region slug %q does not match any known region.", slug)
			}
			return fmt.Sprintf("Region lookup failed for %q.", slug)
		}
	}
	return ""
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
	// region_slugs is optional on the schema since slice α; always
	// emit a non-nil pointer (possibly to an empty array) so the
	// admin response shape stays predictable for downstream consumers.
	regions := nonNilSlice(append([]string(nil), p.RegionSlugs...))
	out := oapi.SubmissionPayload{
		Name:        p.Name,
		ShortDesc:   p.ShortDesc,
		WebsiteUrl:  p.WebsiteURL,
		Tags:        nonNilSlice(append([]string(nil), p.Tags...)),
		RegionSlugs: &regions,
	}
	if p.ContactURL != "" {
		c := p.ContactURL
		out.ContactUrl = &c
	}
	return out
}
