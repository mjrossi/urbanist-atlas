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
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// PromotionEnqueuer is the seam the approve handler uses to hand an
// approved submission off to the GitHub PR worker. Implementations
// must return promptly — they should buffer the job and return nil,
// not block on GitHub I/O. Any error, atlas.ErrPromotionDisabled
// included, is persisted as the submission's promotion_error.
type PromotionEnqueuer interface {
	Enqueue(ctx context.Context, sub atlas.Submission) error
}

// createSubmissionHandler answers POST /api/v1/submissions.
func createSubmissionHandler(subs atlas.SubmissionStore, regions atlas.Store, logger *slog.Logger, m *Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		var body oapi.NewSubmissionRequest
		if perr := decodeOneJSONObject(w, r, &body, "NewSubmissionRequest"); perr != nil {
			m.incSubmissions("rejected_validation")
			writeProblem(w, r, http.StatusBadRequest, problemValidation, perr.title, perr.detail, rid)
			return
		}
		sub, err := atlas.CreateSubmission(r.Context(), regions, subs, fromOAPINewSubmission(body))
		if err != nil {
			writeCreateSubmissionErr(w, r, err, rid, logger, m)
			return
		}
		m.incSubmissions("created")
		writeJSON(w, http.StatusCreated, toOAPISubmission(sub))
	}
}

// writeCreateSubmissionErr maps an atlas.CreateSubmission error onto a
// 400 field-error problem or a 500.
func writeCreateSubmissionErr(w http.ResponseWriter, r *http.Request, err error, rid string, logger *slog.Logger, m *Metrics) {
	var verr *atlas.SubmissionValidationError
	if !errors.As(err, &verr) {
		m.incSubmissions("error")
		logger.ErrorContext(r.Context(), "create submission failed", "err", err, "rid", rid)
		writeInternalProblem(w, r, rid)
		return
	}
	m.incSubmissions("rejected_validation")
	for field := range verr.Fields {
		m.incSubmissionValidationFailure(field)
	}
	writeProblemWithErrors(w, r, http.StatusBadRequest, problemValidation,
		"Submission Validation Failed",
		"One or more fields in the submission failed validation. See the errors map for per-field messages.",
		rid, verr.Fields)
}

// submissionParseError carries the problem-document title+detail for a
// malformed request body. It is returned (not written) so the handler
// stays the single response-writing site and owns the metric increment.
type submissionParseError struct {
	title  string
	detail string
}

func (e *submissionParseError) Error() string { return e.title + ": " + e.detail }

// decodeOneJSONObject decodes exactly one JSON object with no unknown
// fields into v, under submissionBodyLimit. It writes nothing to w, which
// http.MaxBytesReader needs only for its oversize accounting.
func decodeOneJSONObject(w http.ResponseWriter, r *http.Request, v any, typeName string) *submissionParseError {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, submissionBodyLimit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		title := "Invalid Request Body"
		detail := fmt.Sprintf("The request body is not valid JSON for %s.", typeName)
		if errors.Is(err, io.EOF) {
			title = "Empty Request Body"
			detail = fmt.Sprintf("The request body is empty; expected a %s JSON object.", typeName)
		}
		// An oversize body keeps the 400 status: to the client it is a
		// validation failure.
		var mbErr *http.MaxBytesError
		if errors.As(err, &mbErr) {
			title = "Request Body Too Large"
			detail = fmt.Sprintf("The request body exceeds the maximum size of %d bytes.", submissionBodyLimit)
		}
		return &submissionParseError{title: title, detail: detail}
	}
	// Decode stops after the first value, so trailing content needs its
	// own check.
	if dec.More() {
		return &submissionParseError{
			title:  "Invalid Request Body",
			detail: fmt.Sprintf("The request body must contain exactly one %s JSON object.", typeName),
		}
	}
	return nil
}

// listSubmissionsHandler answers GET /api/v1/admin/submissions, pending
// by default. `X-Next-Cursor` carries the token for the next page.
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
		limit, ok := parseLimitParam(w, r, maxAdminListLimit, rid)
		if !ok {
			return
		}
		q.Limit = limit
		if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
			q.Cursor = raw
		}
		page, err := subs.ListPage(r.Context(), q)
		if err != nil {
			if errors.Is(err, atlas.ErrInvalidCursor) {
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
func approveSubmissionHandler(subs atlas.SubmissionStore, enq PromotionEnqueuer, logger *slog.Logger, m *Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		publicID, ok := readSubmissionID(w, r, rid)
		if !ok {
			return
		}
		sub, err := subs.Approve(r.Context(), publicID)
		if err != nil {
			m.incAdminAction("approve", adminOutcome(err))
			writeSubmissionStateErr(w, r, err, rid, logger, "approve submission")
			return
		}

		sub = enqueuePromotion(r.Context(), subs, enq, logger, sub, rid)

		// The approval succeeded even if the enqueue failed; that failure
		// is in promotion_error.
		m.incAdminAction("approve", "ok")
		logger.InfoContext(r.Context(), "submission approved", "public_id", sub.PublicID, "rid", rid)
		writeJSON(w, http.StatusOK, toOAPISubmission(sub))
	}
}

// enqueuePromotion hands an approved submission to the PR worker,
// best-effort: an enqueue failure never undoes the approval. It is
// recorded in promotion_error and the row re-read so the response shows
// it; a failed attach is logged and a failed re-read returns sub as is.
func enqueuePromotion(ctx context.Context, subs atlas.SubmissionStore, enq PromotionEnqueuer, logger *slog.Logger, sub atlas.Submission, rid string) atlas.Submission {
	enqErr := enqueueOrDisabled(ctx, enq, sub)
	if enqErr == nil {
		return sub
	}
	attachErr := subs.AttachPromotionResult(ctx, sub.PublicID, "", enqErr.Error())
	if attachErr != nil {
		logger.ErrorContext(ctx, "attach promotion_error after enqueue failure", "err", attachErr, "rid", rid)
	}
	// Re-read so the response reflects the persisted error.
	if reread, rerr := subs.Get(ctx, sub.PublicID); rerr == nil {
		sub = reread
	}
	return sub
}

// rejectSubmissionHandler answers POST /api/v1/admin/submissions/{id}/reject.
func rejectSubmissionHandler(subs atlas.SubmissionStore, logger *slog.Logger, m *Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		publicID, ok := readSubmissionID(w, r, rid)
		if !ok {
			return
		}
		var body oapi.RejectSubmissionRequest
		if perr := decodeOneJSONObject(w, r, &body, "RejectSubmissionRequest"); perr != nil {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, perr.title, perr.detail, rid)
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
			m.incAdminAction("reject", adminOutcome(err))
			writeSubmissionStateErr(w, r, err, rid, logger, "reject submission")
			return
		}
		m.incAdminAction("reject", "ok")
		logger.InfoContext(r.Context(), "submission rejected", "public_id", sub.PublicID, "rid", rid)
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

// adminOutcome maps a submission state-transition error to a bounded
// metric outcome label, mirroring the cases writeSubmissionStateErr
// switches on. err == nil yields "ok".
func adminOutcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, atlas.ErrSubmissionNotFound):
		return "not_found"
	case errors.Is(err, atlas.ErrSubmissionNotPending):
		return "conflict"
	default:
		return "error"
	}
}

func enqueueOrDisabled(ctx context.Context, enq PromotionEnqueuer, sub atlas.Submission) error {
	if enq == nil {
		return atlas.ErrPromotionDisabled
	}
	return enq.Enqueue(ctx, sub)
}

// fromOAPINewSubmission maps the wire body onto atlas input. Tags are
// not on the wire: editors assign them in the promotion PR.
func fromOAPINewSubmission(body oapi.NewSubmissionRequest) atlas.NewSubmissionInput {
	in := atlas.NewSubmissionInput{
		Payload: atlas.SubmissionPayload{
			Name:       body.Payload.Name,
			ShortDesc:  body.Payload.ShortDesc,
			WebsiteURL: body.Payload.WebsiteUrl,
		},
	}
	if body.Payload.RegionSlugs != nil {
		in.Payload.RegionSlugs = *body.Payload.RegionSlugs
	}
	if body.Payload.ContactUrl != nil {
		in.Payload.ContactURL = *body.Payload.ContactUrl
	}
	if body.SubmitterName != nil {
		in.SubmitterName = *body.SubmitterName
	}
	if body.SubmitterEmail != nil {
		in.SubmitterEmail = string(*body.SubmitterEmail)
	}
	if body.SubmitterNote != nil {
		in.SubmitterNote = *body.SubmitterNote
	}
	return in
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
	out.SubmitterName = strPtrIfSet(s.SubmitterName)
	if s.SubmitterEmail != "" {
		e := openapi_types.Email(s.SubmitterEmail)
		out.SubmitterEmail = &e
	}
	out.SubmitterNote = strPtrIfSet(s.SubmitterNote)
	out.PromotionPrUrl = strPtrIfSet(s.PromotionPRURL)
	out.PromotionError = strPtrIfSet(s.PromotionError)
	out.RejectionReason = strPtrIfSet(s.RejectionReason)
	return out
}

// strPtrIfSet maps an empty string to an omitted optional wire field.
func strPtrIfSet(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toOAPISubmissionPayload(p atlas.SubmissionPayload) oapi.SubmissionPayload {
	// region_slugs is optional in the schema but always emitted, as `[]`
	// when empty, so the admin response shape is stable.
	regions := nonNilSlice(append([]string(nil), p.RegionSlugs...))
	out := oapi.SubmissionPayload{
		Name:        p.Name,
		ShortDesc:   p.ShortDesc,
		WebsiteUrl:  p.WebsiteURL,
		RegionSlugs: &regions,
	}
	out.ContactUrl = strPtrIfSet(p.ContactURL)
	return out
}
