package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// parseLimitParam reads the optional ?limit= query parameter shared by
// the capped list endpoints (admin submissions, region search, coverage
// gaps). An absent parameter returns 0 — callers pass that through so
// the store applies its own default; the cap is endpoint-specific. On a
// non-integer or out-of-range value it writes the 400 problem document
// and returns ok=false, so the caller just returns.
func parseLimitParam(w http.ResponseWriter, r *http.Request, maxLimit int, rid string) (limit int, ok bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxLimit {
		writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Limit",
			fmt.Sprintf("The limit query parameter must be an integer between 1 and %d.", maxLimit), rid)
		return 0, false
	}
	return n, true
}
