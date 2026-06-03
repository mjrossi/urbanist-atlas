package httpapi

import "time"

// Tuning constants for the HTTP layer. Centralized here so an ops
// adjustment lands in one place and so review of "what numbers are we
// running with?" is a one-file read. Each constant carries a comment
// explaining the rationale — change the rationale when you change the
// number.

// rateLimitWindow is the per-IP sliding window for the public
// submission endpoint. Combined with --submissions-rate-per-hour
// (default 5), an IP gets at most N requests in this window before
// being told to back off. One hour matches the flag name and is a
// gentle deterrent — Cloudflare's WAF is the real edge throttle.
const rateLimitWindow = time.Hour

// rateLimitSweepInterval is how often the limiter walks its in-memory
// hit map and discards entries whose hits have all aged out of the
// window. Keeps the map from leaking memory under churning IPs without
// paying a sweep cost on every request. Five minutes is long enough
// that the work amortizes cheaply, short enough that idle-IP entries
// don't linger across multiple windows.
const rateLimitSweepInterval = 5 * time.Minute

// submissionBodyLimit is the maximum bytes we read from
// POST /api/v1/submissions (and the admin reject endpoint's reason
// body, which uses the same cap). The form's typical payload is ~1
// KiB; 64 KiB is generous headroom and small enough that a malicious
// client can't pin a worker on JSON parsing or commit a multi-MB blob
// to disk via http.MaxBytesReader.
const submissionBodyLimit = 64 * 1024

// maxAdminListLimit caps ?limit= on GET /api/v1/admin/submissions.
// Matches the store's internal page cap. Keeps the admin UI responsive
// and pushes deep history through pagination instead of one giant
// response.
const maxAdminListLimit = 200

// maxRegionSearchLimit caps ?limit= on GET /api/v1/regions/search.
// Matches the `maximum: 20` in the OpenAPI spec and the store's own
// hard cap — the type-ahead never needs more than a screenful.
const maxRegionSearchLimit = 20

// Upper bounds on the raw `postal_code` and `country` query parameters
// for GET /api/v1/lookup. Generous relative to the longest validators
// in pkg/atlas/postal.go (PT is 7 chars after hyphen-strip; every
// other supported country is ≤ 7) — they exist to reject pathological
// inputs (multi-MB strings the net/http header cap still allows)
// before NormalizePostalCode allocates.
const (
	maxPostalCodeLen = 16
	maxCountryLen    = 4
)

// requestTimeout bounds a single request's total processing time.
// Applied by timeoutMiddleware so handlers (and the pkg/atlas /
// SQLite calls they forward into) see a cancellable context with
// this deadline. Coarser ReadTimeout/WriteTimeout on http.Server
// still apply; per-request lets a slow downstream cancel cleanly
// instead of waiting for the broader transport budget to expire.
const requestTimeout = 10 * time.Second
