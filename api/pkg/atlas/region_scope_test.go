package atlas

import (
	"slices"
	"testing"
)

func TestBucketOrgsByScope_MetroAndStateMatchIsStatewide(t *testing.T) {
	metro := Region{ID: 1, Kind: "us:metro", Slug: "nyc-metro", ScopeTier: ScopeRegional, SortPriority: 50}
	state := Region{ID: 2, Kind: "us:state", Slug: "ny", ScopeTier: ScopeRegional, SortPriority: 70}
	inScope := map[int64]Region{metro.ID: metro, state.ID: state}
	org := Org{ID: 1, Slug: "both", Name: "Both", Regions: []Region{metro, state}}

	local, regional, statewide := BucketOrgsByScope(inScope, []Org{org})

	// Maintainer decision: an org attached at the state level is presented
	// with its broadest non-local reach, so statewide beats regional.
	if len(local) != 0 || len(regional) != 0 || len(statewide) != 1 {
		t.Fatalf("buckets = local %d, regional %d, statewide %d; want 0, 0, 1",
			len(local), len(regional), len(statewide))
	}
	got := slices.Sorted(slices.Values(statewide[0].MatchedRegionSlugs))
	if want := []string{"ny", "nyc-metro"}; !slices.Equal(got, want) {
		t.Errorf("MatchedRegionSlugs = %v, want %v", got, want)
	}
}
