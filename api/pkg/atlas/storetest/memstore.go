package storetest

import (
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// MemStoreFactory returns a fresh atlas.MemStore for each contract
// test, wired up with a Seeder that delegates to MemStore's
// AddRegion / AddPostalCode / AddOrg helpers. Pass to RunContractSuite
// from a non-integration test file in package atlas_test.
func MemStoreFactory(t *testing.T) (atlas.Store, Seeder, func()) {
	t.Helper()
	s := atlas.NewMemStore()
	return s, memSeeder{s}, func() {}
}

type memSeeder struct{ s *atlas.MemStore }

func (m memSeeder) SeedRegion(tb testing.TB, r atlas.Region) {
	tb.Helper()
	if err := m.s.AddRegion(r); err != nil {
		tb.Fatalf("seed region %q: %v", r.Slug, err)
	}
}

func (m memSeeder) SeedPostalCode(tb testing.TB, country atlas.Country, code string, leafRegionID int64) {
	tb.Helper()
	if err := m.s.AddPostalCode(country, code, leafRegionID); err != nil {
		tb.Fatalf("seed postal code %s %s: %v", country, code, err)
	}
}

func (m memSeeder) SeedOrg(_ testing.TB, org atlas.Org, regionIDs []int64) {
	m.s.AddOrg(org, regionIDs)
}

func (m memSeeder) SeedRollupState(_ testing.TB, metroSlug, stateSlug string) {
	m.s.AddRollupState(metroSlug, stateSlug)
}
