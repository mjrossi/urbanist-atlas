package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// listCoverageGapsHandler answers GET /api/v1/admin/coverage-gaps.
// Bearer-gated. Returns recent sampled empty-result lookups/searches
// newest-first, capped at ?limit= (default 50, max 200). Thin adapter:
// parse limit → call the reader → encode.
func listCoverageGapsHandler(reader atlas.CoverageGapReader, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		limit := 0 // 0 lets the store apply its default; the cap is shared
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > maxAdminListLimit {
				writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Limit",
					fmt.Sprintf("The limit query parameter must be an integer between 1 and %d.", maxAdminListLimit), rid)
				return
			}
			limit = n
		}
		gaps, err := reader.ListCoverageGaps(r.Context(), limit)
		if err != nil {
			logger.ErrorContext(r.Context(), "list coverage gaps failed", "err", err, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		out := make([]oapi.CoverageGap, 0, len(gaps))
		for _, g := range gaps {
			out = append(out, toOAPICoverageGap(g))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func toOAPICoverageGap(g atlas.CoverageGap) oapi.CoverageGap {
	out := oapi.CoverageGap{
		Kind:      oapi.CoverageGapKind(g.Kind),
		Input:     g.Input,
		CreatedAt: g.CreatedAt,
	}
	// country is omitted for search gaps (no country axis).
	if g.Country != "" {
		c := g.Country
		out.Country = &c
	}
	return out
}
