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
		"us:city",
		"us:county",
		"us:multi-state",
		"us:borough",
		"us:transit-federation",
		"ca:province",
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

// TestMetroKinds_Deterministic confirms the accessor returns the kinds
// in a stable, alphabetical order. The SQL layer passes this slice as a
// $1::text[] parameter, and deterministic ordering makes for
// deterministic query plans (and easier-to-read EXPLAINs).
func TestMetroKinds_Deterministic(t *testing.T) {
	want := []RegionKind{
		"ca:cma",
		"ca:regional-district",
		"pt:area-metropolitana",
		"us:metro",
	}
	got := MetroKinds()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("MetroKinds() (-want +got):\n%s", diff)
	}

	// Call again — same order. Defensive against an accidental
	// map-iteration leak that would happen to pass on the first call.
	got2 := MetroKinds()
	if diff := cmp.Diff(got, got2); diff != "" {
		t.Errorf("MetroKinds() not stable across calls (-first +second):\n%s", diff)
	}
}
