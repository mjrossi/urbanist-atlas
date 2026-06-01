package seedfiles

import (
	"strings"
	"testing"
	"testing/fstest"
)

// countrySpec is a copy of the unexported per-country loader entry,
// re-declared in the test so fixtures can drive buildMemStore over a
// tiny synthetic country set instead of the production US/CA bundle.
// (The production struct is anonymous, so the test mirrors its fields.)

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

const emptyOrgs = "" // orgs.toml may be empty (zero [[org]] tables)

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

// TestBuildMemStore_CleanFixtureLoads confirms buildMemStore wires the
// global cycle + orphan checks WITHOUT false-positiving on a valid
// acyclic, fully-anchored fixture: every leaf has a postal anchor and
// the graph is a clean DAG.
func TestBuildMemStore_CleanFixtureLoads(t *testing.T) {
	regionTOML := map[string]string{
		"u_top":  region("State", "u:state", "regional", nil),
		"u_mid":  region("Metro", "u:metro", "regional", []string{"State"}),
		"u_leaf": region("City", "u:leaf", "local", []string{"Metro"}),
	}
	postal := postalHeaderCSV() + "10001,U,city\n"
	fs := fixtureFS(regionTOML, map[string]string{"u": postal}, emptyOrgs)

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
		"y_top": region("State", "y:state", "regional", nil),
		// anchored has a postal row → fine. orphan has nothing.
		"y_leaves": region("Anchored", "y:leaf", "local", []string{"State"}) +
			region("Orphan", "y:leaf", "local", []string{"State"}),
	}
	postal := postalHeaderCSV() + "10001,Y,anchored\n"
	fs := fixtureFS(regionTOML, map[string]string{"y": postal}, emptyOrgs)

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
		"z_top": region("State", "z:state", "regional", nil),
		"z_leaves": region("ByPostal", "z:leaf", "local", []string{"State"}) +
			region("ByOrg", "z:leaf", "local", []string{"State"}),
	}
	postal := postalHeaderCSV() + "10001,Z,bypostal\n"
	orgs := `[[org]]
slug = "advocates"
name = "Advocates"
short_desc = "x"
website_url = "https://example.org"
tags = ["transit"]
region_slugs = ["byorg"]
added_at = 2026-01-01
`
	fs := fixtureFS(regionTOML, map[string]string{"z": postal}, orgs)

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
		"w_top": region("State", "w:state", "regional", nil),
		// metro is a non-leaf (city is its child); city carries the
		// postal anchor. metro itself has no postal row but is anchored
		// transitively, and is not a leaf so isn't directly checked.
		"w_mid":  region("Metro", "w:metro", "regional", []string{"State"}),
		"w_leaf": region("City", "w:leaf", "local", []string{"Metro"}),
	}
	postal := postalHeaderCSV() + "10001,W,city\n"
	fs := fixtureFS(regionTOML, map[string]string{"w": postal}, emptyOrgs)

	cs := []countrySpec{{Code: "W", RegionFiles: []string{"w_top", "w_mid", "w_leaf"}, Postal: "w"}}
	if _, err := buildMemStore(nil, fs, cs); err != nil {
		t.Fatalf("descendant-anchored region should not be an orphan: %v", err)
	}
}

// TestDetectCycles_WithinFileEarlySignal confirms the per-file fast
// early signal is retained: a cycle entirely within one file is caught
// by DetectCycles before the global pass even runs.
func TestDetectCycles_WithinFileEarlySignal(t *testing.T) {
	body := region("A", "x:leaf", "local", []string{"b"}) +
		region("B", "x:leaf", "local", []string{"a"})
	regions, err := ParseRegions(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if err := DetectCycles(regions); err == nil {
		t.Fatal("within-file DetectCycles did not catch the same-file cycle")
	}
}

// region renders a single [[region]] TOML table body for a fixture.
func region(name, kind, scope string, parents []string) string {
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
