package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// dayParamFormat is the YYYY-MM-DD form of the from/to query params and
// of usage_daily.day.
const dayParamFormat = "2006-01-02"

// maxUsageLimit caps GET /api/v1/admin/usage. Higher than the
// submission/coverage list cap because the digest legitimately pulls a
// few hundred buckets per month in one call.
const maxUsageLimit = 1000

// listUsageHandler answers GET /api/v1/admin/usage. Bearer-gated.
// Returns accumulated daily usage buckets, highest-count first.
//
// from and to are required: an unbounded range would scan the entire
// rollup table, and every real caller (the digest workflow) knows the
// month it wants.
func listUsageHandler(reader atlas.UsageReader, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		q := r.URL.Query()

		from, ok := parseDayParam(w, r, q.Get("from"), "from", rid)
		if !ok {
			return
		}
		to, ok := parseDayParam(w, r, q.Get("to"), "to", rid)
		if !ok {
			return
		}
		limit, ok := parseLimitParam(w, r, maxUsageLimit, rid)
		if !ok {
			return
		}

		rows, err := reader.ListUsage(r.Context(), from, to, q.Get("kind"), limit)
		if err != nil {
			logger.ErrorContext(r.Context(), "list usage failed", "err", err, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		out := make([]oapi.UsageCount, 0, len(rows))
		for _, c := range rows {
			converted, convErr := toOAPIUsageCount(c)
			if convErr != nil {
				// A stored day that won't parse means the table was
				// written by something other than the recorder. Loud,
				// because it breaks the digest silently otherwise.
				logger.ErrorContext(r.Context(), "usage row has unparseable day",
					"err", convErr, "day", c.Day, "kind", c.Kind, "rid", rid)
				writeInternalProblem(w, r, rid)
				return
			}
			out = append(out, converted)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// parseDayParam validates a required YYYY-MM-DD query param, writing a
// 400 problem document and returning ok=false when it is missing or
// malformed.
func parseDayParam(w http.ResponseWriter, r *http.Request, raw, name, rid string) (string, bool) {
	if raw == "" {
		writeProblem(w, r, http.StatusBadRequest, problemValidation, "Missing Parameter",
			"The "+name+" query parameter is required (format: YYYY-MM-DD).", rid)
		return "", false
	}
	if _, err := time.Parse(dayParamFormat, raw); err != nil {
		writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Parameter",
			"The "+name+" query parameter must be a date in YYYY-MM-DD form.", rid)
		return "", false
	}
	return raw, true
}

// toOAPIUsageCount adapts a store row to the wire shape. The generated
// Day field is an openapi_types.Date (the spec declares format: date),
// so the stored 'YYYY-MM-DD' string is parsed rather than assigned.
func toOAPIUsageCount(c atlas.UsageCount) (oapi.UsageCount, error) {
	day, err := time.Parse(dayParamFormat, c.Day)
	if err != nil {
		return oapi.UsageCount{}, err
	}
	return oapi.UsageCount{
		Day:   openapi_types.Date{Time: day},
		Kind:  oapi.UsageCountKind(c.Kind),
		Key:   c.Key,
		Count: c.Count,
	}, nil
}
