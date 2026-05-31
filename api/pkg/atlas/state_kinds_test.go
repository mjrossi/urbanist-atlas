package atlas

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestIsStateKind asserts the editorial set of state-equivalent kinds.
// The list is the source of truth in state_kinds.go; this test is the
// regression guard so a future country addition can't silently fold a
// state-equivalent into Regional (or vice versa).
func TestIsStateKind(t *testing.T) {
	in := []RegionKind{
		"us:state",
		"ca:province",
		// Territories are the top-admin tier of their country and carry
		// internal regional structure (PR is the parent of six metros),
		// so a territory-wide org belongs in the Statewide tier, above
		// any single metro. Canada colloquially groups these with
		// provinces ("provinces and territories").
		"us:territory",
		"ca:territory",
	}
	out := []RegionKind{
		// Sub-state regional kinds — stay in Regional.
		"us:metro",
		"ca:cma",
		"ca:regional-district",
		"us:transit-federation",
		"pt:area-metropolitana",
		"pt:distrito",
		// Multi-state coalitions — broader than a state but an advocacy
		// federation, not a top-admin tier; editorial ruling keeps them
		// in Regional.
		"us:multi-state",
		// DC (us:federal-district) is a city-state: the district is
		// coextensive with one city and one metro, and the seed already
		// splits it across two nodes (washington-dc, a us:city local
		// leaf, and dc, the district). City-scale DC orgs tag the local
		// leaf; DMV-scale orgs tag the metro. Promoting the kind here
		// would yank DMV-tagged orgs (e.g. Greater Greater Washington)
		// into "State / Provincial", so it's deliberately excluded.
		"us:federal-district",
		// Local-tier kinds.
		"us:city",
		"us:county",
		"us:borough",
		"ca:city",
		"pt:freguesia",
		"pt:municipio",
		// de:land IS state-equivalent, but it is intentionally NOT in
		// the v1 set (DE hasn't shipped). When it is added, city-states
		// (Berlin/Hamburg/Bremen) are still shielded by the Local
		// precedence in BucketOrgsByScope, not by this set.
		"de:land",
		// National tier.
		"pt:nacional",
		"",
	}
	for _, k := range in {
		if !IsStateKind(k) {
			t.Errorf("IsStateKind(%q) = false, want true", k)
		}
	}
	for _, k := range out {
		if IsStateKind(k) {
			t.Errorf("IsStateKind(%q) = true, want false", k)
		}
	}
}

// TestStateKinds_Deterministic confirms the accessor returns the kinds
// in a stable, alphabetical order across calls — defensive against an
// accidental map-iteration order leak.
func TestStateKinds_Deterministic(t *testing.T) {
	want := []RegionKind{
		"ca:province",
		"ca:territory",
		"us:state",
		"us:territory",
	}
	got := StateKinds()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("StateKinds() (-want +got):\n%s", diff)
	}

	got2 := StateKinds()
	if diff := cmp.Diff(got, got2); diff != "" {
		t.Errorf("StateKinds() not stable across calls (-first +second):\n%s", diff)
	}
}
