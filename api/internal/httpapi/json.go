package httpapi

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
