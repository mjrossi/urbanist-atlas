// Package postgres provides a Postgres-backed implementation of
// pkg/atlas.Store against the region-graph schema introduced in
// migration 0002. It is a thin adapter over sqlc-generated query
// functions in the gen subpackage; business logic stays in pkg/atlas.
//
// Lookup is answered with at most three round-trips per call:
//   - ResolveLeafRegion (1 row), AncestorRegions (recursive CTE),
//     ParentSlugsForRegions to hydrate Region.ParentSlugs.
//   - OrgsForRegionsAndAllRegionIDs + GetRegionsByIDs + a second
//     ParentSlugsForRegions to hydrate each Org.Regions.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/postgres/gen"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

type Store struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: gen.New(pool)}
}

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

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ResolveLeafRegion implements atlas.Store.
func (s *Store) ResolveLeafRegion(ctx context.Context, country atlas.Country, postalCode string) (atlas.Region, error) {
	normalized := atlas.NormalizePostalCode(country, postalCode)
	row, err := s.q.ResolveLeafRegion(ctx, gen.ResolveLeafRegionParams{
		Country:    string(country),
		PostalCode: normalized,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return atlas.Region{}, atlas.ErrPostalCodeNotFound
		}
		return atlas.Region{}, fmt.Errorf("postgres: resolve leaf region: %w", err)
	}
	r := atlas.Region{
		ID:           row.ID,
		Country:      atlas.Country(row.Country),
		Kind:         atlas.RegionKind(row.Kind),
		Name:         row.Name,
		Slug:         row.Slug,
		ScopeTier:    atlas.ScopeTier(row.ScopeTier),
		SortPriority: int(row.SortPriority),
	}
	parents, err := s.parentSlugsByRegion(ctx, []int64{r.ID})
	if err != nil {
		return atlas.Region{}, err
	}
	r.ParentSlugs = parents[r.ID]
	return r, nil
}

// AncestorRegions implements atlas.Store.
func (s *Store) AncestorRegions(ctx context.Context, leafRegionID int64) ([]atlas.Region, error) {
	rows, err := s.q.AncestorRegions(ctx, leafRegionID)
	if err != nil {
		return nil, fmt.Errorf("postgres: ancestor regions: %w", err)
	}
	ids := make([]int64, len(rows))
	regions := make([]atlas.Region, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
		regions[i] = atlas.Region{
			ID:           row.ID,
			Country:      atlas.Country(row.Country),
			Kind:         atlas.RegionKind(row.Kind),
			Name:         row.Name,
			Slug:         row.Slug,
			ScopeTier:    atlas.ScopeTier(row.ScopeTier),
			SortPriority: int(row.SortPriority),
		}
	}
	parents, err := s.parentSlugsByRegion(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range regions {
		regions[i].ParentSlugs = parents[regions[i].ID]
	}
	return regions, nil
}

// OrgsForRegions implements atlas.Store.
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
	seen := map[int64]struct{}{}
	for _, row := range rows {
		for _, rid := range row.RegionIds {
			seen[rid] = struct{}{}
		}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	regionsByID, err := s.regionsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	parents, err := s.parentSlugsByRegion(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, r := range regionsByID {
		r.ParentSlugs = parents[id]
		regionsByID[id] = r
	}
	out := make([]atlas.Org, 0, len(rows))
	for _, row := range rows {
		regions := make([]atlas.Region, 0, len(row.RegionIds))
		for _, rid := range row.RegionIds {
			if r, ok := regionsByID[rid]; ok {
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
			ID:           r.ID,
			Country:      atlas.Country(r.Country),
			Kind:         atlas.RegionKind(r.Kind),
			Name:         r.Name,
			Slug:         r.Slug,
			ScopeTier:    atlas.ScopeTier(r.ScopeTier),
			SortPriority: int(r.SortPriority),
		}
	}
	return out, nil
}

// ListMetros, GetMetro, and ListRecent are placeholders until the
// sqlc-generated browse queries land in Phase 4 + the adapter in
// Phase 5. They satisfy the widened atlas.Store interface so the
// build stays green at every commit.

// ListMetros implements atlas.Store. (Postgres-backed implementation
// lands in a follow-up phase.)
func (s *Store) ListMetros(ctx context.Context) ([]atlas.MetroSummary, error) {
	return nil, errors.New("postgres: ListMetros not yet implemented")
}

// GetMetro implements atlas.Store. (Postgres-backed implementation
// lands in a follow-up phase.)
func (s *Store) GetMetro(ctx context.Context, slug string) (*atlas.MetroDetail, error) {
	return nil, errors.New("postgres: GetMetro not yet implemented")
}

// ListRecent implements atlas.Store. (Postgres-backed implementation
// lands in a follow-up phase.)
func (s *Store) ListRecent(ctx context.Context) ([]atlas.Org, error) {
	return nil, errors.New("postgres: ListRecent not yet implemented")
}

func (s *Store) parentSlugsByRegion(ctx context.Context, ids []int64) (map[int64][]string, error) {
	out := make(map[int64][]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ParentSlugsForRegions(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("postgres: parent slugs: %w", err)
	}
	for _, r := range rows {
		out[r.RegionID] = append(out[r.RegionID], r.ParentSlug)
	}
	return out, nil
}

var _ atlas.Store = (*Store)(nil)
