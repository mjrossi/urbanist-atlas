package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// encodeJSON is the single JSON encode site for responses. The status
// is already written, so a failure (almost always a client that hung up)
// can only be logged, at DEBUG because it is noise rather than a fault.
func encodeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelDebug,
			"response encode failed", slog.Any("err", err))
	}
}

// nonNilSlice returns s, or an empty slice when s is nil, so the wire
// carries `[]` rather than `null` for an empty collection.
func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
