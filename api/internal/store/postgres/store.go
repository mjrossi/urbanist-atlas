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
	"github.com/jackc/pgx/v5/pgtype"
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

// ListMetros implements atlas.Store. The SQL is in browse.sql; this
// adapter only fills the metro-kind parameter, maps rows to the domain
// type, and hydrates Region.ParentSlugs for each row.
func (s *Store) ListMetros(ctx context.Context) ([]atlas.MetroSummary, error) {
	rows, err := s.q.ListMetros(ctx, atlas.MetroKindStrings())
	if err != nil {
		return nil, fmt.Errorf("postgres: list metros: %w", err)
	}
	if len(rows) == 0 {
		return []atlas.MetroSummary{}, nil
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	parents, err := s.parentSlugsByRegion(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]atlas.MetroSummary, len(rows))
	for i, r := range rows {
		out[i] = atlas.MetroSummary{
			Region: atlas.Region{
				ID:           r.ID,
				Country:      atlas.Country(r.Country),
				Kind:         atlas.RegionKind(r.Kind),
				Name:         r.Name,
				Slug:         r.Slug,
				ScopeTier:    atlas.ScopeTier(r.ScopeTier),
				SortPriority: int(r.SortPriority),
				ParentSlugs:  parents[r.ID],
			},
			OrgCount: r.OrgCount,
		}
	}
	return out, nil
}

// GetMetro implements atlas.Store. Returns (nil, nil) for unknown
// slugs and for known slugs that don't name a metro-equivalent region
// (the SQL gates on both conditions). Orgs are newest-first.
func (s *Store) GetMetro(ctx context.Context, slug string) (*atlas.MetroDetail, error) {
	row, err := s.q.GetMetroBySlug(ctx, gen.GetMetroBySlugParams{Slug: slug, Kinds: atlas.MetroKindStrings()})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: get metro: %w", err)
	}
	region := atlas.Region{
		ID:           row.ID,
		Country:      atlas.Country(row.Country),
		Kind:         atlas.RegionKind(row.Kind),
		Name:         row.Name,
		Slug:         row.Slug,
		ScopeTier:    atlas.ScopeTier(row.ScopeTier),
		SortPriority: int(row.SortPriority),
	}
	parents, err := s.parentSlugsByRegion(ctx, []int64{region.ID})
	if err != nil {
		return nil, err
	}
	region.ParentSlugs = parents[region.ID]

	orgRows, err := s.q.OrgsForMetro(ctx, region.ID)
	if err != nil {
		return nil, fmt.Errorf("postgres: orgs for metro: %w", err)
	}
	orgs, err := s.hydrateOrgRows(ctx, orgsForMetroRows(orgRows))
	if err != nil {
		return nil, err
	}
	return &atlas.MetroDetail{Region: region, Orgs: orgs}, nil
}

// GetOrgBySlug implements atlas.Store. Returns atlas.ErrOrgNotFound for
// unknown slugs and for slugs that name a non-approved org (the SQL
// gates on status='approved'). The row shape matches OrgsForMetro /
// ListRecent, so hydration goes through the shared hydrateOrgRows path.
func (s *Store) GetOrgBySlug(ctx context.Context, slug string) (*atlas.Org, error) {
	row, err := s.q.GetOrgBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, atlas.ErrOrgNotFound
		}
		return nil, fmt.Errorf("postgres: get org by slug: %w", err)
	}
	orgs, err := s.hydrateOrgRows(ctx, []orgRow{{
		ID: row.ID, Slug: row.Slug, Name: row.Name, ShortDesc: row.ShortDesc,
		WebsiteUrl: row.WebsiteUrl, ContactUrl: row.ContactUrl, Tags: row.Tags,
		CreatedAt: row.CreatedAt, RegionIds: row.RegionIds,
	}})
	if err != nil {
		return nil, err
	}
	if len(orgs) == 0 {
		return nil, atlas.ErrOrgNotFound
	}
	return &orgs[0], nil
}

// ListRecent implements atlas.Store. The SQL handles the national-tier
// filter, ordering, and 10-row cap; this adapter only maps rows and
// hydrates Org.Regions.
func (s *Store) ListRecent(ctx context.Context) ([]atlas.Org, error) {
	rows, err := s.q.ListRecent(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list recent: %w", err)
	}
	return s.hydrateOrgRows(ctx, listRecentRows(rows))
}

// orgRow normalizes the two sqlc-generated row types that carry the
// same columns (ListRecentRow, OrgsForMetroRow). The adapter walks the
// slice once to gather distinct region IDs, hydrates them in one
// round-trip, then maps each row to an atlas.Org. Keeping the row
// types in a tiny internal interface avoids two near-identical copies
// of the hydration code.
type orgRow struct {
	ID         int64
	Slug       string
	Name       string
	ShortDesc  string
	WebsiteUrl string
	ContactUrl pgtype.Text
	Tags       []string
	CreatedAt  pgtype.Timestamptz
	RegionIds  []int64
}

func orgsForMetroRows(rows []gen.OrgsForMetroRow) []orgRow {
	out := make([]orgRow, len(rows))
	for i, r := range rows {
		out[i] = orgRow{
			ID: r.ID, Slug: r.Slug, Name: r.Name, ShortDesc: r.ShortDesc,
			WebsiteUrl: r.WebsiteUrl, ContactUrl: r.ContactUrl, Tags: r.Tags,
			CreatedAt: r.CreatedAt, RegionIds: r.RegionIds,
		}
	}
	return out
}

func listRecentRows(rows []gen.ListRecentRow) []orgRow {
	out := make([]orgRow, len(rows))
	for i, r := range rows {
		out[i] = orgRow{
			ID: r.ID, Slug: r.Slug, Name: r.Name, ShortDesc: r.ShortDesc,
			WebsiteUrl: r.WebsiteUrl, ContactUrl: r.ContactUrl, Tags: r.Tags,
			CreatedAt: r.CreatedAt, RegionIds: r.RegionIds,
		}
	}
	return out
}

func (s *Store) hydrateOrgRows(ctx context.Context, rows []orgRow) ([]atlas.Org, error) {
	if len(rows) == 0 {
		return []atlas.Org{}, nil
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
		if row.CreatedAt.Valid {
			org.CreatedAt = row.CreatedAt.Time
		}
		out = append(out, org)
	}
	return out, nil
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
