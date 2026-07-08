package atlas

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestIsDefaultBrowseKind asserts the editorial default-browse set
// — the kinds the `/api/v1/regions` list returns. Source of truth
// lives in browse_kinds.go; this test catches accidental demotion
// of a previously-included kind.
func TestIsDefaultBrowseKind(t *testing.T) {
	in := []RegionKind{
		"us:metro",
		"us:city",
		"ca:cma",
		"ca:regional-district",
		"ca:city",
		"pt:area-metropolitana",
	}
	// Out of the default set — these don't appear in /regions today.
	// A future filter slice (taxonomy- or DAG-based) could surface
	// them; the editorial default stays metros + cities until then.
	out := []RegionKind{
		"us:state",
		"us:federal-district",
		"us:territory",
		"us:county",
		"us:multi-state",
		"us:borough",
		"us:transit-federation",
		"ca:province",
		"ca:territory",
		"pt:distrito",
		"pt:nuts-ii",
		"pt:nacional",
		"pt:freguesia",
		"pt:municipio",
		"",
	}
	for _, k := range in {
		if !IsDefaultBrowseKind(k) {
			t.Errorf("IsDefaultBrowseKind(%q) = false, want true", k)
		}
	}
	for _, k := range out {
		if IsDefaultBrowseKind(k) {
			t.Errorf("IsDefaultBrowseKind(%q) = true, want false", k)
		}
	}
}

// TestDefaultBrowseKinds_ExactSet pins the full membership of the
// editorial default-browse set, so an accidental addition (which the
// in/out lists above can't catch) fails loudly too.
func TestDefaultBrowseKinds_ExactSet(t *testing.T) {
	want := map[RegionKind]bool{
		"us:metro":              true,
		"us:city":               true,
		"ca:cma":                true,
		"ca:regional-district":  true,
		"ca:city":               true,
		"pt:area-metropolitana": true,
	}
	if diff := cmp.Diff(want, defaultBrowseKinds); diff != "" {
		t.Errorf("defaultBrowseKinds (-want +got):\n%s", diff)
	}
}

// TestDefaultBrowseKinds_IsSupersetOfMetroKinds pins the editorial
// relationship between the two predicates: every metro-equivalent
// kind also appears in the default browse set. The reverse is not
// true (cities are in the default but aren't metro-equivalent for
// /lookup label purposes).
func TestDefaultBrowseKinds_IsSupersetOfMetroKinds(t *testing.T) {
	for k := range metroKinds {
		if !IsDefaultBrowseKind(k) {
			t.Errorf("IsDefaultBrowseKind(%q) = false, but kind is in metroKinds — defaultBrowseKinds must be a superset", k)
		}
	}
}
