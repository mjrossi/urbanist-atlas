package atlas

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestIsMetroKind asserts the editorial set of metro-equivalent kinds.
// The list is the source of truth in metro_kinds.go; this test is the
// regression guard so a future country addition can't silently demote a
// previously-supported kind.
func TestIsMetroKind(t *testing.T) {
	in := []RegionKind{
		"us:metro",
		"ca:cma",
		"ca:regional-district",
		"pt:area-metropolitana",
	}
	out := []RegionKind{
		"us:state",
		"us:federal-district",
		"us:territory",
		"us:city",
		"us:county",
		"us:multi-state",
		"us:borough",
		"us:transit-federation",
		"ca:province",
		"ca:territory",
		"ca:city",
		"pt:distrito",
		"pt:nuts-ii",
		"pt:nacional",
		"pt:freguesia",
		"pt:municipio",
		"",
	}
	for _, k := range in {
		if !IsMetroKind(k) {
			t.Errorf("IsMetroKind(%q) = false, want true", k)
		}
	}
	for _, k := range out {
		if IsMetroKind(k) {
			t.Errorf("IsMetroKind(%q) = true, want false", k)
		}
	}
}

// TestMetroKinds_ExactSet pins the full membership of the editorial
// metro-equivalent set, so an accidental addition (which the in/out
// lists above can't catch) fails loudly too.
func TestMetroKinds_ExactSet(t *testing.T) {
	want := map[RegionKind]bool{
		"us:metro":              true,
		"ca:cma":                true,
		"ca:regional-district":  true,
		"pt:area-metropolitana": true,
	}
	if diff := cmp.Diff(want, metroKinds); diff != "" {
		t.Errorf("metroKinds (-want +got):\n%s", diff)
	}
}
