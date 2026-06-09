package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// encodeJSON streams v to w as JSON. It is the single encode site
// behind writeJSON, writeProblemWithErrors, and respondCollection so
// the "what happens when the encode fails" policy lives in exactly one
// place.
//
// By the time Encode runs the caller has already set the content-type
// and called WriteHeader, so the status line is on the wire and there
// is nothing left to do but observe the failure. An encode error here
// is almost always a client that hung up mid-response (broken pipe),
// which is operational noise rather than a server fault — so it is
// logged on the default logger at DEBUG (a breadcrumb available when
// someone turns Debug on, not a steady-state alert). The error is no
// longer silently discarded.
func encodeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelDebug,
			"response encode failed", slog.Any("err", err))
	}
}

// nonNilSlice returns s when non-nil and a zero-length slice of the
// same element type otherwise. Used at every JSON-rendering site that
// needs to emit `[]` instead of `null` for an absent collection — Go's
// encoding/json marshals a nil slice as `null`, which surprises web
// clients that expect a stable array shape (length 0 is fine; null
// blows up `forEach`/destructuring).
//
// Generic so call sites keep their typed slice (e.g. []oapi.Region)
// without an `any` conversion. Cheap allocation: only when s is nil.
func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
