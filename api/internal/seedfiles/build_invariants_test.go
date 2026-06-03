package seedfiles

import (
	"strings"
	"testing"
	"testing/fstest"
)

// These tests live in the white-box seedfiles package so they can call
// the unexported buildMemStore core with a tiny synthetic countrySpec
// set, exercising the global cross-file acyclicity (RGN-02a) and
// orphan/zero-anchor (RGN-02b) checks without the full US/CA bundle.

// fixtureFS assembles an in-memory seed bundle: one region TOML per
// suffix, one postal CSV per country code, and orgs.toml. Each value
// in regionTOML is the full file body.
func fixtureFS(regionTOML map[string]string, postalCSV map[string]string, orgsTOML string) fstest.MapFS {
	fs := fstest.MapFS{}
	for suffix, body := range regionTOML {
		fs["regions_"+suffix+".toml"] = &fstest.MapFile{Data: []byte(body)}
	}
	for code, body := range postalCSV {
		fs["postal_codes_"+code+".csv"] = &fstest.MapFile{Data: []byte(body)}
	}
	fs["orgs.toml"] = &fstest.MapFile{Data: []byte(orgsTOML)}
	return fs
}

// orgFor renders a minimal valid orgs.toml attaching one org to the
// given region slug. ParseOrgs requires at least one org, so every
// fixture needs one; pointing it at an already-anchored region keeps
// it out of the way of the orphan-leaf assertions.
func orgFor(regionSlug string) string {
	return `[[org]]
slug = "advocates"
name = "Advocates"
short_desc = "x"
website_url = "https://example.org"
tags = ["transit"]
region_slugs = ["` + regionSlug + `"]
added_at = 2026-01-01
`
}

// TestDetectCyclesGraph_CrossFileCycle proves RGN-02a at the algorithm
// level: the generalized detector walks EVERY parent edge in an
// assembled parents map (no inFile skip), so a cross-file back-edge
// (A → B → A spanning two files) is caught. The per-file DetectCycles
// misses this because it skips edges whose parent is defined elsewhere.
func TestDetectCyclesGraph_CrossFileCycle(t *testing.T) {
	// Assembled graph as BuildMemStore would build it from all files:
	// S has no parent; A parents S and B; B parents S and A. The A↔B
	// edge is the cross-file cycle.
	parents := map[string][]string{
		"s": nil,
		"a": {"s", "b"},
		"b": {"s", "a"},
	}
	err := DetectCyclesGraph(parents)
	if err == nil {
		t.Fatal("expected cross-file cycle error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "cycle") {
		t.Fatalf("error %q does not mention a cycle", err)
	}
}

// TestDetectCyclesGraph_Acyclic confirms a normal leaf→tier DAG (the
// cross-file shape that is legitimate) passes the global detector.
func TestDetectCyclesGraph_Acyclic(t *testing.T) {
	parents := map[string][]string{
		"state": nil,
		"metro": {"state"},
		"city":  {"metro"},
	}
	if err := DetectCyclesGraph(parents); err != nil {
		t.Fatalf("acyclic graph rejected: %v", err)
	}
}

// TestDetectCyclesGraph_DiamondCyclePathFidelity pins the human-readable
// cycle path on a multi-parent (diamond) graph: the reported chain names
// only the genuine cycle plus its lead-in, never a slug from an unrelated
// sibling branch.
//
// This locks the error-path FORMAT — the contract the defensive path
// copy in detectCycles3Color protects. It is deliberately NOT a WR-02
// regression repro: the slice aliasing WR-02 guards against is not
// observable through this DFS (it formats the error immediately on the
// gray-revisit and overwrites shared tail indices on descent first), so
// this test passes with or without the copy — verified by reverting the
// copy and sweeping 17 diamond+cycle shapes (0 leaks). Its value is
// catching a FUTURE refactor that genuinely corrupts the reported path
// (e.g. retaining path slices to collect every cycle rather than
// returning on the first).
func TestDetectCyclesGraph_DiamondCyclePathFidelity(t *testing.T) {
	// metro is the diamond node. One parent leads to an acyclic sibling
	// branch (sibling-region → sibling-parent); the other (state) closes
	// the cycle metro → state → metro. city is an acyclic lead-in. The
	// sibling branch is explored first (slice order), so a path-aliasing
	// bug would be most likely to leak one of its slugs here.
	parents := map[string][]string{
		"city":           {"metro"},
		"metro":          {"sibling-region", "state"},
		"sibling-region": {"sibling-parent"},
		"sibling-parent": nil,
		"state":          {"metro"},
	}
	err := DetectCyclesGraph(parents)
	if err == nil {
		t.Fatal("expected diamond cycle error, got nil")
	}
	got := err.Error()
	if !strings.Contains(strings.ToLower(got), "cycle") {
		t.Fatalf("error %q does not mention a cycle", got)
	}
	// The genuine cycle members appear, in order.
	if !strings.Contains(got, "metro → state → metro") {
		t.Errorf("reported path missing the genuine cycle chain:\n%s", got)
	}
	// No slug from the unrelated sibling branch leaks into the path.
	for _, foreign := range []string{"sibling-region", "sibling-parent"} {
		if strings.Contains(got, foreign) {
			t.Errorf("sibling-branch slug %q leaked into cycle path:\n%s", foreign, got)
		}
	}
}

// TestBuildMemStore_CleanFixtureLoads confirms buildMemStore wires the
// global cycle + orphan checks WITHOUT false-positiving on a valid
// acyclic, fully-anchored fixture: every leaf has a postal anchor and
// the graph is a clean DAG.
func TestBuildMemStore_CleanFixtureLoads(t *testing.T) {
	regionTOML := map[string]string{
		"u_top":  regionTable("State", "u:state", "regional", nil),
		"u_mid":  regionTable("Metro", "u:metro", "regional", []string{"State"}),
		"u_leaf": regionTable("City", "u:leaf", "local", []string{"Metro"}),
	}
	postal := postalHeaderCSV() + "10001,U,city\n"
	fs := fixtureFS(regionTOML, map[string]string{"u": postal}, orgFor("city"))

	cs := []countrySpec{{Code: "U", RegionFiles: []string{"u_top", "u_mid", "u_leaf"}, Postal: "u"}}
	if _, err := buildMemStore(nil, fs, cs); err != nil {
		t.Fatalf("clean fixture rejected: %v", err)
	}
}

// TestBuildMemStore_OrphanLeaf proves RGN-02b: a leaf region with no
// postal anchor, no attached org, and no anchoring descendant fails the
// build, and the error names the orphan slug.
func TestBuildMemStore_OrphanLeaf(t *testing.T) {
	regionTOML := map[string]string{
		"y_top": regionTable("State", "y:state", "regional", nil),
		// anchored has a postal row → fine. orphan has nothing.
		"y_leaves": regionTable("Anchored", "y:leaf", "local", []string{"State"}) +
			regionTable("Orphan", "y:leaf", "local", []string{"State"}),
	}
	postal := postalHeaderCSV() + "10001,Y,anchored\n"
	fs := fixtureFS(regionTOML, map[string]string{"y": postal}, orgFor("anchored"))

	cs := []countrySpec{{Code: "Y", RegionFiles: []string{"y_top", "y_leaves"}, Postal: "y"}}
	_, err := buildMemStore(nil, fs, cs)
	if err == nil {
		t.Fatal("expected orphan-leaf error, got nil")
	}
	if !strings.Contains(err.Error(), "orphan") {
		t.Fatalf("error %q does not name the orphaned slug %q", err, "orphan")
	}
}

// TestBuildMemStore_OrphanAnchoredByOrg proves the org-attachment leg
// of the RGN-02b reachability rule: a leaf with no postal row but with
// an attached org is NOT an orphan.
func TestBuildMemStore_OrphanAnchoredByOrg(t *testing.T) {
	regionTOML := map[string]string{
		"z_top": regionTable("State", "z:state", "regional", nil),
		"z_leaves": regionTable("ByPostal", "z:leaf", "local", []string{"State"}) +
			regionTable("ByOrg", "z:leaf", "local", []string{"State"}),
	}
	postal := postalHeaderCSV() + "10001,Z,bypostal\n"
	fs := fixtureFS(regionTOML, map[string]string{"z": postal}, orgFor("byorg"))

	cs := []countrySpec{{Code: "Z", RegionFiles: []string{"z_top", "z_leaves"}, Postal: "z"}}
	if _, err := buildMemStore(nil, fs, cs); err != nil {
		t.Fatalf("org-anchored leaf should not be an orphan: %v", err)
	}
}

// TestBuildMemStore_OrphanAnchoredByDescendant proves the
// anchoring-descendant leg: a non-leaf region whose only anchor is a
// descendant's postal row is fine, and only true leaves are checked.
func TestBuildMemStore_OrphanAnchoredByDescendant(t *testing.T) {
	regionTOML := map[string]string{
		"w_top": regionTable("State", "w:state", "regional", nil),
		// metro is a non-leaf (city is its child); city carries the
		// postal anchor. metro itself has no postal row but is anchored
		// transitively, and is not a leaf so isn't directly checked.
		"w_mid":  regionTable("Metro", "w:metro", "regional", []string{"State"}),
		"w_leaf": regionTable("City", "w:leaf", "local", []string{"Metro"}),
	}
	postal := postalHeaderCSV() + "10001,W,city\n"
	fs := fixtureFS(regionTOML, map[string]string{"w": postal}, orgFor("city"))

	cs := []countrySpec{{Code: "W", RegionFiles: []string{"w_top", "w_mid", "w_leaf"}, Postal: "w"}}
	if _, err := buildMemStore(nil, fs, cs); err != nil {
		t.Fatalf("descendant-anchored region should not be an orphan: %v", err)
	}
}

// TestDetectCycles_WithinFileEarlySignal confirms the per-file fast
// early signal is retained: a cycle entirely within one file is caught
// by DetectCycles before the global pass even runs.
func TestDetectCycles_WithinFileEarlySignal(t *testing.T) {
	body := regionTable("A", "x:leaf", "local", []string{"b"}) +
		regionTable("B", "x:leaf", "local", []string{"a"})
	regions, err := ParseRegions(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if err := DetectCycles(regions); err == nil {
		t.Fatal("within-file DetectCycles did not catch the same-file cycle")
	}
}

// regionTable renders a single [[region]] TOML table body for a
// fixture. (Distinct from the black-box seedfiles_test `region` helper
// in added_at_test.go, which returns a one-row *fstest.MapFile.)
func regionTable(name, kind, scope string, parents []string) string {
	var b strings.Builder
	slug := strings.ToLower(name)
	b.WriteString("[[region]]\n")
	b.WriteString("slug = \"" + slug + "\"\n")
	b.WriteString("kind = \"" + kind + "\"\n")
	b.WriteString("name = \"" + name + "\"\n")
	b.WriteString("scope_tier = \"" + scope + "\"\n")
	if len(parents) > 0 {
		quoted := make([]string, len(parents))
		for i, p := range parents {
			quoted[i] = "\"" + strings.ToLower(p) + "\""
		}
		b.WriteString("parents = [" + strings.Join(quoted, ", ") + "]\n")
	}
	b.WriteString("\n")
	return b.String()
}

func postalHeaderCSV() string { return "postal_code,country,leaf_region_slug\n" }

// rollupFixture builds a two-file bundle: a top region (the rollup
// target) and a stateless metro carrying `rollup_states = [target]`. The
// metro is regional-tier so it never trips the local-leaf orphan check;
// the org attaches to the metro so ParseOrgs is satisfied.
func rollupFixture(targetSlug, targetKind, rollupTarget string) fstest.MapFS {
	top := "[[region]]\nslug = \"" + targetSlug + "\"\nkind = \"" + targetKind +
		"\"\nname = \"Target\"\nscope_tier = \"regional\"\nsort_priority = 60\n"
	metro := "[[region]]\nslug = \"chicago-metro\"\nkind = \"us:metro\"\nname = \"Chicago Metro\"\n" +
		"scope_tier = \"regional\"\nsort_priority = 40\nrollup_states = [\"" + rollupTarget + "\"]\n"
	postal := postalHeaderCSV() + "10001,R," + targetSlug + "\n"
	return fixtureFS(
		map[string]string{"r_top": top, "r_metro": metro},
		map[string]string{"r": postal},
		orgFor("chicago-metro"),
	)
}

func rollupCountrySpec() []countrySpec {
	return []countrySpec{{Code: "R", RegionFiles: []string{"r_top", "r_metro"}, Postal: "r"}}
}

// TestBuildMemStore_RollupStatesValid confirms a metro naming a
// state-equivalent region in rollup_states loads cleanly (the directional
// edge resolves and never enters the parent/cycle graph).
func TestBuildMemStore_RollupStatesValid(t *testing.T) {
	fs := rollupFixture("il", "us:state", "il")
	if _, err := buildMemStore(nil, fs, rollupCountrySpec()); err != nil {
		t.Fatalf("valid rollup_states fixture rejected: %v", err)
	}
}

// TestBuildMemStore_RollupStatesFederalDistrictAccepted pins the DC
// relaxation: us:federal-district is a valid rollup target even though it
// is not a state-equivalent kind for bucketing (IsRollupTargetKind).
func TestBuildMemStore_RollupStatesFederalDistrictAccepted(t *testing.T) {
	fs := rollupFixture("dc", "us:federal-district", "dc")
	if _, err := buildMemStore(nil, fs, rollupCountrySpec()); err != nil {
		t.Fatalf("federal-district rollup target should be accepted: %v", err)
	}
}

// TestBuildMemStore_RollupStatesUnknownTarget fails closed when
// rollup_states names a slug no region defines.
func TestBuildMemStore_RollupStatesUnknownTarget(t *testing.T) {
	fs := rollupFixture("il", "us:state", "nope")
	_, err := buildMemStore(nil, fs, rollupCountrySpec())
	if err == nil || !strings.Contains(err.Error(), "rollup_states references unknown slug") {
		t.Fatalf("want unknown-slug rollup error, got %v", err)
	}
}

// TestBuildMemStore_RollupStatesNonStateKind fails closed when
// rollup_states points at a non-state, non-federal-district kind (here a
// metro) — guarding against an editor pointing a rollup at the wrong tier.
func TestBuildMemStore_RollupStatesNonStateKind(t *testing.T) {
	// Target is a us:metro (registered, but not a valid rollup kind).
	fs := rollupFixture("other-metro", "us:metro", "other-metro")
	_, err := buildMemStore(nil, fs, rollupCountrySpec())
	if err == nil || !strings.Contains(err.Error(), "must be a state-equivalent") {
		t.Fatalf("want non-state-kind rollup error, got %v", err)
	}
}
