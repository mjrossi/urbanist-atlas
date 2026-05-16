// Package postgres provides a Postgres-backed implementation of
// pkg/atlas.Store. It is a thin adapter over sqlc-generated query
// functions in the gen subpackage; business logic stays in pkg/atlas.
//
// The adapter normalizes postal codes in Go (uppercase, strip
// whitespace, truncate Canadian codes to their 3-character FSA) using
// the same rules as MemStore, then runs typed queries against pgx.
//
// Lookup is answered with at most two round-trips per call:
//   - one ResolvePostalCode + one GetRegionsByIDs to materialize the
//     ResolvedPostalCode, OR
//   - one OrgsForRegionsAndAllRegionIDs + one GetRegionsByIDs to
//     materialize each Org.Regions.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/postgres/gen"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Store implements pkg/atlas.Store against a Postgres database.
type Store struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// New returns a Store that issues queries against the given pgx pool.
// The caller owns the pool's lifecycle.
func New(pool *pgxpool.Pool) *Store {
	return &Store{
		pool: pool,
		q:    gen.New(pool),
	}
}

// Open is a convenience that builds a pool from a connection string
// and returns a Store. The returned Close func closes the underlying
// pool; callers should call it on shutdown.
func Open(ctx context.Context, dbURL string) (*Store, func(), error) {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return New(pool), pool.Close, nil
}

// Pool exposes the underlying pgx pool. Useful for tests that need to
// hand-insert rows without going through the adapter, and for future
// queries that don't fit the sqlc model (e.g. ad-hoc admin work).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ResolvePostalCode implements atlas.Store. The code argument is the
// user's raw input; it's normalized here (uppercase, strip whitespace,
// truncate CA codes to FSA) using the same rules as MemStore so the two
// stores answer the same queries the same way.
func (s *Store) ResolvePostalCode(ctx context.Context, country atlas.Country, postalCode string) (atlas.ResolvedPostalCode, error) {
	normalized := normalizePostalCode(country, postalCode)
	row, err := s.q.ResolvePostalCode(ctx, gen.ResolvePostalCodeParams{
		Country:    string(country),
		PostalCode: normalized,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return atlas.ResolvedPostalCode{}, atlas.ErrPostalCodeNotFound
		}
		return atlas.ResolvedPostalCode{}, fmt.Errorf("postgres: resolve postal code: %w", err)
	}

	// Collect the non-null region IDs and hydrate them in one round-trip.
	ids := make([]int64, 0, 4)
	if row.CityRegionID.Valid {
		ids = append(ids, row.CityRegionID.Int64)
	}
	if row.CountyRegionID.Valid {
		ids = append(ids, row.CountyRegionID.Int64)
	}
	if row.MetroRegionID.Valid {
		ids = append(ids, row.MetroRegionID.Int64)
	}
	if row.StateRegionID.Valid {
		ids = append(ids, row.StateRegionID.Int64)
	}
	byID, err := s.regionsByID(ctx, ids)
	if err != nil {
		return atlas.ResolvedPostalCode{}, err
	}

	rpc := atlas.ResolvedPostalCode{
		Code:    row.PostalCode,
		Country: atlas.Country(row.Country),
	}
	if row.CityRegionID.Valid {
		if r, ok := byID[row.CityRegionID.Int64]; ok {
			rpc.City = &r
		}
	}
	if row.CountyRegionID.Valid {
		if r, ok := byID[row.CountyRegionID.Int64]; ok {
			rpc.County = &r
		}
	}
	if row.MetroRegionID.Valid {
		if r, ok := byID[row.MetroRegionID.Int64]; ok {
			rpc.Metro = &r
		}
	}
	if row.StateRegionID.Valid {
		if r, ok := byID[row.StateRegionID.Int64]; ok {
			rpc.State = &r
		}
	}
	return rpc, nil
}

// OrgsForRegions implements atlas.Store. Each returned Org has its
// full Regions slice populated — every region the org serves, not just
// the ones that matched the query.
func (s *Store) OrgsForRegions(ctx context.Context, regionIDs []int64) ([]atlas.Org, error) {
	if len(regionIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q.OrgsForRegionsAndAllRegionIDs(ctx, regionIDs)
	if err != nil {
		return nil, fmt.Errorf("postgres: orgs for regions: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// Collect every region ID we'll need to hydrate Org.Regions for the
	// returned orgs.
	seen := make(map[int64]struct{})
	for _, row := range rows {
		for _, rid := range row.RegionIds {
			seen[rid] = struct{}{}
		}
	}
	allIDs := make([]int64, 0, len(seen))
	for rid := range seen {
		allIDs = append(allIDs, rid)
	}
	byID, err := s.regionsByID(ctx, allIDs)
	if err != nil {
		return nil, err
	}

	out := make([]atlas.Org, 0, len(rows))
	for _, row := range rows {
		regions := make([]atlas.Region, 0, len(row.RegionIds))
		for _, rid := range row.RegionIds {
			if r, ok := byID[rid]; ok {
				regions = append(regions, r)
			}
		}
		tags := make([]atlas.Tag, len(row.Tags))
		for i, t := range row.Tags {
			tags[i] = atlas.Tag(t)
		}
		org := atlas.Org{
			ID:         row.ID,
			Slug:       row.Slug,
			Name:       row.Name,
			ShortDesc:  row.ShortDesc,
			WebsiteURL: row.WebsiteUrl,
			Tags:       tags,
			Regions:    regions,
		}
		if row.ContactUrl.Valid {
			org.ContactURL = row.ContactUrl.String
		}
		out = append(out, org)
	}
	return out, nil
}

// regionsByID hydrates a set of region IDs into a map keyed by ID.
// Returns an empty map (never nil) if ids is empty.
func (s *Store) regionsByID(ctx context.Context, ids []int64) (map[int64]atlas.Region, error) {
	out := make(map[int64]atlas.Region, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.GetRegionsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("postgres: get regions: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = atlas.Region{
			ID:        r.ID,
			Kind:      atlas.RegionKind(r.Kind),
			Name:      r.Name,
			Slug:      r.Slug,
			Country:   atlas.Country(r.Country),
			ScopeTier: atlas.ScopeTier(r.ScopeTier),
		}
	}
	return out, nil
}

// normalizePostalCode applies the same canonicalization rules MemStore
// uses (see pkg/atlas/memstore.go). Encoded in Go rather than SQL so
// the two stores can't drift, and so we never store junk in the DB.
//
// Canadian inputs are truncated to the first three characters (FSA),
// since FSA is the granularity at which we resolve regions for CA.
func normalizePostalCode(country atlas.Country, code string) string {
	c := strings.ToUpper(strings.ReplaceAll(code, " ", ""))
	if country == atlas.CountryCA && len(c) > 3 {
		c = c[:3]
	}
	return c
}

// Compile-time check that Store satisfies atlas.Store.
var _ atlas.Store = (*Store)(nil)
