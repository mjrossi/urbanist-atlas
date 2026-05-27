//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas/storetest"
)

// TestPostgresStore_Contract runs the shared Store contract suite
// against the Postgres adapter. Boots one testcontainer for the suite
// and truncates tables between contracts so each contract starts from
// a clean slate (cheaper than a fresh container per contract).
func TestPostgresStore_Contract(t *testing.T) {
	store, closeFn := startPostgres(t)
	t.Cleanup(closeFn)

	factory := func(t *testing.T) (atlas.Store, storetest.Seeder, func()) {
		t.Helper()
		truncateAll(t, store)
		return store, pgSeeder{store}, func() {}
	}
	storetest.RunContractSuite(t, factory)
}

func truncateAll(t *testing.T, store *Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stmt := `TRUNCATE
		organization_regions,
		organizations,
		postal_codes,
		region_parents,
		regions
		RESTART IDENTITY CASCADE`
	if _, err := store.Pool().Exec(ctx, stmt); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

type pgSeeder struct{ s *Store }

func (p pgSeeder) SeedRegion(t *testing.T, r atlas.Region) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := p.s.Pool()
	_, err := pool.Exec(ctx, `
		INSERT INTO regions (id, kind, name, slug, country, scope_tier, sort_priority)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.ID, string(r.Kind), r.Name, r.Slug, string(r.Country), string(r.ScopeTier), r.SortPriority,
	)
	if err != nil {
		t.Fatalf("seed region %s: %v", r.Slug, err)
	}
	for _, ps := range r.ParentSlugs {
		_, err := pool.Exec(ctx, `
			INSERT INTO region_parents (region_id, parent_region_id)
			SELECT $1, id FROM regions WHERE slug = $2`,
			r.ID, ps)
		if err != nil {
			t.Fatalf("seed parent edge %s -> %s: %v", r.Slug, ps, err)
		}
	}
}

func (p pgSeeder) SeedPostalCode(t *testing.T, country atlas.Country, code string, leafRegionID int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := p.s.Pool().Exec(ctx, `
		INSERT INTO postal_codes (postal_code, country, leaf_region_id)
		VALUES ($1, $2, $3)`,
		atlas.NormalizePostalCode(country, code), string(country), leafRegionID,
	)
	if err != nil {
		t.Fatalf("seed postal code %s/%s: %v", country, code, err)
	}
}

func (p pgSeeder) SeedOrg(t *testing.T, org atlas.Org, regionIDs []int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := p.s.Pool()

	// Use the provided CreatedAt when non-zero; otherwise let the
	// default NOW() fire (matches production org seeding).
	tags := make([]string, len(org.Tags))
	for i, t := range org.Tags {
		tags[i] = string(t)
	}
	var contactURL any
	if org.ContactURL != "" {
		contactURL = org.ContactURL
	}
	if org.CreatedAt.IsZero() {
		_, err := pool.Exec(ctx, `
			INSERT INTO organizations
				(id, slug, name, short_desc, website_url, contact_url, tags, status, approved_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'approved', NOW())`,
			org.ID, org.Slug, org.Name, org.ShortDesc, org.WebsiteURL, contactURL, tags,
		)
		if err != nil {
			t.Fatalf("seed org %s: %v", org.Slug, err)
		}
	} else {
		_, err := pool.Exec(ctx, `
			INSERT INTO organizations
				(id, slug, name, short_desc, website_url, contact_url, tags, status, created_at, approved_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'approved', $8, $8)`,
			org.ID, org.Slug, org.Name, org.ShortDesc, org.WebsiteURL, contactURL, tags, org.CreatedAt,
		)
		if err != nil {
			t.Fatalf("seed org %s: %v", org.Slug, err)
		}
	}
	for _, rid := range regionIDs {
		_, err := pool.Exec(ctx, `
			INSERT INTO organization_regions (organization_id, region_id)
			VALUES ($1, $2)`,
			org.ID, rid,
		)
		if err != nil {
			t.Fatalf("seed org-region edge %d/%d: %v", org.ID, rid, err)
		}
	}
}
