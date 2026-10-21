package httpapi

import (
	"net/http"
)

// Stable problem-type URIs, mirroring the catalog in openapi.yaml's
// ProblemDetails schema.
const (
	problemValidation    = "https://urbanistatlas.com/problems/validation"
	problemNotFound      = "https://urbanistatlas.com/problems/not-found"
	problemMilitaryZIP   = "https://urbanistatlas.com/problems/military-postal-code"
	problemInternal      = "https://urbanistatlas.com/problems/internal"
	problemUnauthorized  = "https://urbanistatlas.com/problems/unauthorized"
	problemConflict      = "https://urbanistatlas.com/problems/conflict"
	problemRateLimited   = "https://urbanistatlas.com/problems/rate-limited"
	problemAdminDisabled = "https://urbanistatlas.com/problems/admin-disabled"
)

// writeJSON writes a success response; errors go through writeProblem.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encodeJSON(w, body)
}

// problemBody is an RFC 9457 problem document. It is hand-written rather
// than oapi.ProblemDetails so it can carry the `errors` extension member
// (RFC 9457 §3.2).
type problemBody struct {
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Status    int               `json:"status"`
	Detail    string            `json:"detail,omitempty"`
	Instance  string            `json:"instance,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	Errors    map[string]string `json:"errors,omitempty"`
}

// writeInternalProblem writes the standard 500 problem document.
func writeInternalProblem(w http.ResponseWriter, r *http.Request, rid string) {
	writeProblem(w, r, http.StatusInternalServerError, problemInternal,
		"Internal Server Error",
		"Something went wrong on our end. Please try again in a moment.", rid)
}

// writeProblem writes an application/problem+json response. requestID
// becomes the spec's `request_id` extension.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, typeURI, title, detail, requestID string) {
	writeProblemWithErrors(w, r, status, typeURI, title, detail, requestID, nil)
}

// writeProblemWithErrors is writeProblem plus the `errors` extension, a
// `{field: message}` map for per-field form errors. An empty map is
// omitted.
func writeProblemWithErrors(w http.ResponseWriter, r *http.Request, status int, typeURI, title, detail, requestID string, fieldErrors map[string]string) {
	body := problemBody{
		Type:      typeURI,
		Title:     title,
		Status:    status,
		Detail:    detail,
		RequestID: requestID,
		Errors:    fieldErrors,
	}
	if r != nil {
		body.Instance = r.URL.Path
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	encodeJSON(w, body)
}
