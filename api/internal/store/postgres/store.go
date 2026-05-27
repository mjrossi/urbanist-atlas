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

// ListRegions implements atlas.Store. The SQL is in browse.sql;
// this adapter passes the default browse kind set as the @kinds
// parameter, maps rows to the domain type, and hydrates
// Region.ParentSlugs for each row. The SQL stays parameterized so a
// future filter slice can plumb a kinds arg through without
// rewriting the query.
func (s *Store) ListRegions(ctx context.Context) ([]atlas.RegionSummary, error) {
	rows, err := s.q.ListRegions(ctx, atlas.DefaultBrowseKindStrings())
	if err != nil {
		return nil, fmt.Errorf("postgres: list regions: %w", err)
	}
	if len(rows) == 0 {
		return []atlas.RegionSummary{}, nil
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	parents, err := s.parentSlugsByRegion(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]atlas.RegionSummary, len(rows))
	for i, r := range rows {
		browseParent := ""
		if r.BrowseParentSlug.Valid {
			browseParent = r.BrowseParentSlug.String
		}
		out[i] = atlas.RegionSummary{
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
			OrgCount:         r.OrgCount,
			BrowseParentSlug: browseParent,
		}
	}
	return out, nil
}

// GetRegion implements atlas.Store. Returns (nil, nil) for unknown
// slugs and for national-tier regions (the SQL gates on both).
// Resolves any non-national region — metros, cities, counties,
// boroughs, states, multi-state coalitions.
//
// Builds a "lookup-style scope" for the focus region: walks both
// ancestors (upward) and descendants (downward), combines them into
// an in-scope region set, fetches all orgs with at least one
// attachment in scope, and buckets by scope_tier via the shared
// pkg/atlas helper (same rule /lookup uses). Result: clicking a
// region from Browse shows the same advocate list a postal-code
// lookup in that region would show.
func (s *Store) GetRegion(ctx context.Context, slug string) (*atlas.RegionDetail, error) {
	row, err := s.q.GetRegionBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: get region: %w", err)
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

	// Upward walk (reuses the /lookup helper). AncestorRegions
	// includes the focus at index 0 and filters national-tier.
	ancestors, err := s.AncestorRegions(ctx, region.ID)
	if err != nil {
		return nil, fmt.Errorf("postgres: ancestors for region: %w", err)
	}

	// Downward walk. DescendantRegions includes the focus at index 0
	// (matches AncestorRegions' contract for symmetry).
	descRows, err := s.q.DescendantRegions(ctx, region.ID)
	if err != nil {
		return nil, fmt.Errorf("postgres: descendants for region: %w", err)
	}

	// Combine into an in-scope map keyed by region ID.
	inScope := make(map[int64]atlas.Region, len(ancestors)+len(descRows))
	for _, r := range ancestors {
		inScope[r.ID] = r
	}
	for _, r := range descRows {
		if r.ScopeTier == string(atlas.ScopeNational) {
			continue
		}
		if _, ok := inScope[r.ID]; ok {
			continue
		}
		inScope[r.ID] = atlas.Region{
			ID:           r.ID,
			Country:      atlas.Country(r.Country),
			Kind:         atlas.RegionKind(r.Kind),
			Name:         r.Name,
			Slug:         r.Slug,
			ScopeTier:    atlas.ScopeTier(r.ScopeTier),
			SortPriority: int(r.SortPriority),
		}
	}

	// Fetch every org with at least one attachment in scope, hydrate
	// their Regions (the bucketing helper reads each org's full
	// attachment list).
	ids := make([]int64, 0, len(inScope))
	for k := range inScope {
		ids = append(ids, k)
	}
	orgs, err := s.OrgsForRegions(ctx, ids)
	if err != nil {
		return nil, err
	}
	local, regional := atlas.BucketOrgsByScope(inScope, orgs)

	// Build the breadcrumb-friendly ancestry slice: closest-first,
	// excluding self and national-tier (the /lookup walk includes
	// self at [0] and pre-filters national).
	ancestry := make([]atlas.Region, 0, len(ancestors))
	for i, r := range ancestors {
		if i == 0 {
			continue
		}
		ancestry = append(ancestry, r)
	}

	return &atlas.RegionDetail{
		Region:   region,
		Local:    local,
		Regional: regional,
		Ancestry: ancestry,
	}, nil
}

// GetOrgBySlug implements atlas.Store. Returns atlas.ErrOrgNotFound for
// unknown slugs and for slugs that name a non-approved org (the SQL
// gates on status='approved'). The row shape matches
// OrgsForRegionsAndAllRegionIDs / ListRecent, so hydration goes
// through the shared hydrateOrgRows path.
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
		// hydrateOrgRows cannot drop rows for a single-row input today.
		// If a future change ever does (e.g. region-tier filtering),
		// surface the integrity violation as 500 rather than masking it
		// as a 404 for an org that actually exists in the table.
		return nil, fmt.Errorf("postgres: hydrateOrgRows dropped row for slug %q", slug)
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
// same columns (ListRecentRow + OrgsForRegionsAndAllRegionIDsRow,
// the latter via OrgsForRegions). The adapter walks the
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
