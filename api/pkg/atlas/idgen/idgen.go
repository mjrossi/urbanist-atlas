// Package idgen produces the public IDs used for submission rows. The
// wrapper is a test seam: callers depend on Generator rather than on
// google/uuid, so tests can inject deterministic IDs.
package idgen

import "github.com/google/uuid"

// Generator returns a new public ID string on each call.
type Generator func() (string, error)

// NewUUIDv7 returns a Generator of UUIDv7 strings, whose millisecond
// timestamp prefix makes IDs sort roughly by creation time.
func NewUUIDv7() Generator {
	return func() (string, error) {
		id, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		return id.String(), nil
	}
}
