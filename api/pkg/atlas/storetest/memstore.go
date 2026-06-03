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

func (m memSeeder) SeedRegion(_ *testing.T, r atlas.Region) {
	m.s.AddRegion(r)
}

func (m memSeeder) SeedPostalCode(_ *testing.T, country atlas.Country, code string, leafRegionID int64) {
	m.s.AddPostalCode(country, code, leafRegionID)
}

func (m memSeeder) SeedOrg(_ *testing.T, org atlas.Org, regionIDs []int64) {
	m.s.AddOrg(org, regionIDs)
}

func (m memSeeder) SeedRollupState(_ *testing.T, metroSlug, stateSlug string) {
	m.s.AddRollupState(metroSlug, stateSlug)
}
