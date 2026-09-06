package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// dayParamFormat is the YYYY-MM-DD form of the from/to query params and
// of usage_daily.day.
const dayParamFormat = "2006-01-02"

// usageKinds is the accepted set of the kind query param, in spec order.
// Kept for the 400 message; membership itself is decided by the
// generated enum's Valid so the spec stays the single source of truth.
var usageKinds = []string{
	string(oapi.ListUsageParamsKindRegionView),
	string(oapi.ListUsageParamsKindOrgView),
	string(oapi.ListUsageParamsKindLookup),
	string(oapi.ListUsageParamsKindLookupTier),
	string(oapi.ListUsageParamsKindLookupResult),
	string(oapi.ListUsageParamsKindLookupCountry),
}

// listUsageHandler answers GET /api/v1/admin/usage. Bearer-gated.
// Returns accumulated usage buckets, highest-count first.
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
		// Zero-padded YYYY-MM-DD sorts chronologically as a string, which
		// is also why the SQL BETWEEN works on the stored TEXT column.
		if from > to {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Range",
				"The from query parameter must not be later than to.", rid)
			return
		}
		kind, ok := parseUsageKindParam(w, r, q.Get("kind"), rid)
		if !ok {
			return
		}
		groupBy, ok := parseUsageGroupByParam(w, r, q.Get("group_by"), rid)
		if !ok {
			return
		}
		limit, ok := parseLimitParam(w, r, atlas.MaxUsageLimit, rid)
		if !ok {
			return
		}

		rows, err := reader.ListUsage(r.Context(), atlas.UsageQuery{
			From:    from,
			To:      to,
			Kind:    kind,
			GroupBy: groupBy,
			Limit:   limit,
		})
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

// parseUsageKindParam validates the optional kind filter. An unknown
// kind is rejected rather than passed through: the CHECK constraint on
// usage_daily.kind guarantees it would match nothing, so a typo would
// otherwise return an empty 200 that reads as "no traffic".
func parseUsageKindParam(w http.ResponseWriter, r *http.Request, raw, rid string) (string, bool) {
	kind := strings.TrimSpace(raw)
	if kind == "" {
		return "", true
	}
	if !oapi.ListUsageParamsKind(kind).Valid() {
		writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Parameter",
			"The kind query parameter must be one of: "+strings.Join(usageKinds, ", ")+".", rid)
		return "", false
	}
	return kind, true
}

// parseUsageGroupByParam validates the optional group_by param,
// defaulting to the range-aggregated read.
func parseUsageGroupByParam(w http.ResponseWriter, r *http.Request, raw, rid string) (atlas.UsageGroupBy, bool) {
	g := strings.TrimSpace(raw)
	if g == "" {
		return atlas.UsageGroupByKey, true
	}
	if !oapi.ListUsageParamsGroupBy(g).Valid() {
		writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Parameter",
			"The group_by query parameter must be one of: key, day.", rid)
		return "", false
	}
	return atlas.UsageGroupBy(g), true
}

// toOAPIUsageCount adapts a store row to the wire shape. Day is a
// pointer because a range-aggregated row spans the whole request window
// and has no single day; the store leaves it empty in that case.
func toOAPIUsageCount(c atlas.UsageCount) (oapi.UsageCount, error) {
	out := oapi.UsageCount{
		Kind:  oapi.UsageCountKind(c.Kind),
		Key:   c.Key,
		Count: c.Count,
	}
	if c.Day == "" {
		return out, nil
	}
	day, err := time.Parse(dayParamFormat, c.Day)
	if err != nil {
		return oapi.UsageCount{}, err
	}
	out.Day = &openapi_types.Date{Time: day}
	return out, nil
}
