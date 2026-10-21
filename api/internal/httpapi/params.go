package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// parseLimitParam reads the optional ?limit=. Absent is 0, which the
// store treats as its default. An invalid value gets a 400 written here
// and ok=false.
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
