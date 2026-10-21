package httpapi

import "time"

// Tuning constants for the HTTP layer. Change the reason with the number.

// rateLimitWindow matches the --submissions-rate-per-hour flag.
const rateLimitWindow = time.Hour

// submissionsGlobalPerWindow caps many-IP clients, far above organic volume.
const submissionsGlobalPerWindow = 30

// maxPendingSubmissions must not exceed the store's List cap (200), or the
// queue can never look full. Full, it is at most 200 × submissionBodyLimit
// (~12.5 MiB) of the 1 GiB volume.
const maxPendingSubmissions = 200

// pendingQueueCheckInterval bounds overshoot to one interval of global limit.
const pendingQueueCheckInterval = time.Minute

// The pending-queue query runs under the cache lock, so it gets a short
// timeout, and a failed query is retried sooner than the normal interval.
const (
	pendingQueueRetryInterval = 5 * time.Second
	pendingQueueQueryTimeout  = 2 * time.Second
)

// rateLimitSweepInterval is cheap to amortize yet short next to the window.
const rateLimitSweepInterval = 5 * time.Minute

// submissionBodyLimit is generous next to the form's ~1 KiB payload.
const submissionBodyLimit = 64 * 1024

// maxAdminListLimit matches the store's page cap.
const maxAdminListLimit = 200

// maxRegionSearchLimit matches the spec's `maximum: 20` and the store's cap.
const maxRegionSearchLimit = 20

// maxRegionSearchQueryLen is in runes, matching the spec's `maxLength: 100`.
const maxRegionSearchQueryLen = 100

// Lookup input caps, generous next to a 10-char ZIP+4, reject junk early.
const (
	maxPostalCodeLen = 16
	maxCountryLen    = 4
)

// requestTimeout must stay under the http.Server WriteTimeout set in
// cmd/server/serve.go (15s), so the context cancels before the write
// deadline cuts the connection.
const requestTimeout = 10 * time.Second
