package atlas

import (
	"context"
	"errors"
)

// ErrPostalCodeNotFound is returned by Store.ResolvePostalCode when no
// row exists for the (country, postal code) pair. The HTTP layer maps
// this to a 404 with a helpful body so the SPA can suggest a nearby
// code or a submission.
var ErrPostalCodeNotFound = errors.New("atlas: postal code not found")

// Store is the persistence seam between pkg/atlas and the rest of the
// system. The interface is intentionally minimal — two operations
// compose to satisfy Lookup. Postgres-backed implementations can
// optimize internally (e.g. a single query with joins) without
// changing the contract.
//
// All implementations must be safe for concurrent use.
type Store interface {
	// ResolvePostalCode returns the geographic regions a postal code
	// belongs to. The code argument should be the user's input; the
	// implementation normalizes (uppercase, whitespace removed,
	// Canadian postal codes truncated to FSA). Returns
	// ErrPostalCodeNotFound if no match exists.
	ResolvePostalCode(ctx context.Context, country Country, postalCode string) (ResolvedPostalCode, error)

	// OrgsForRegions returns all approved organizations tagged with
	// any of the given region IDs. Each returned Org has its full
	// Regions slice populated (the regions the org serves, not just
	// the ones that matched). Order is unspecified — Lookup sorts the
	// results.
	OrgsForRegions(ctx context.Context, regionIDs []int64) ([]Org, error)
}
