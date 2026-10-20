package storetest

import (
	"fmt"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// fatalRecorder stands in for the test a seeder fails, so the failure
// can be observed instead of ending this test.
type fatalRecorder struct {
	testing.TB
	fatals []string
}

func (f *fatalRecorder) Helper() {}

func (f *fatalRecorder) Fatalf(format string, args ...any) {
	f.fatals = append(f.fatals, fmt.Sprintf(format, args...))
}

func TestMemSeeder_FailsOnRejectedFixture(t *testing.T) {
	_, seed, _ := MemStoreFactory(t)

	rec := &fatalRecorder{TB: t}
	seed.SeedRegion(rec, atlas.Region{ID: 1, Slug: "orphan", ParentSlugs: []string{"missing"}})
	if len(rec.fatals) != 1 {
		t.Errorf("SeedRegion with unknown parent: %d fatals, want 1", len(rec.fatals))
	}

	rec = &fatalRecorder{TB: t}
	seed.SeedRegion(rec, atlas.Region{ID: 2, Slug: "a"})
	seed.SeedRegion(rec, atlas.Region{ID: 3, Slug: "b"})
	seed.SeedPostalCode(rec, atlas.CountryUS, "11217", 2)
	seed.SeedPostalCode(rec, atlas.CountryUS, "11217", 3)
	if len(rec.fatals) != 1 {
		t.Errorf("SeedPostalCode remapping a code: %d fatals, want 1 (got %q)", len(rec.fatals), rec.fatals)
	}
}
