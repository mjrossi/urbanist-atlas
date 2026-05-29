// Package idgen produces the public IDs used for submission rows. The
// wrapper exists so callers depend on a test seam rather than on
// google/uuid directly, which keeps the import surface narrow and
// lets tests inject deterministic IDs.
package idgen

import "github.com/google/uuid"

// Generator returns a new public ID string on each call. The
// production implementation returns a UUIDv7 (time-ordered, so DB
// indexes stay tight). A test can inject a fake Generator that
// returns canned values.
type Generator func() (string, error)

// NewUUIDv7 returns a production Generator that produces UUIDv7
// strings via google/uuid. UUIDv7 has a millisecond-precision
// timestamp prefix, so rows insert in approximately created-at order
// and the public IDs sort naturally newest-last.
func NewUUIDv7() Generator {
	return func() (string, error) {
		id, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		return id.String(), nil
	}
}

// MustGenerate is a convenience for call sites where the generator's
// error is treated as a server-internal panic-worthy condition (UUIDv7
// only errors when the system entropy source fails, which is a
// shutdown-class problem).
func MustGenerate(g Generator) string {
	id, err := g()
	if err != nil {
		panic("idgen: generator failed: " + err.Error())
	}
	return id
}
