package us

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCrosswalkHUDBackfill_PicksMaxTotRatio(t *testing.T) {
	// ZIP 20811 spans two synthetic counties; the row with the
	// higher TOT_RATIO wins, even though its RES_RATIO is 0 (the
	// P.O. Box case).
	huds := []HUDZipCounty{
		{ZIP: "20811", County: "24033", ResRatio: 0.0, TotRatio: 0.10},
		{ZIP: "20811", County: "24031", ResRatio: 0.0, TotRatio: 0.90},
	}
	zctaAnchors := []PostalAnchor{} // no existing ZCTA coverage
	countyToMSA := map[string]string{
		"24031": "47900",
		"24033": "47900",
	}
	msaSlugs := map[string]string{
		"47900": "washington-dc-metro",
	}
	got, _ := CrosswalkHUDBackfill(huds, zctaAnchors, countyToMSA, msaSlugs, nil)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %+v", len(got), got)
	}
	if got[0].PostalCode != "20811" || got[0].AnchorSlug != "washington-dc-metro" || got[0].Reason != "hud:msa" {
		t.Errorf("got %+v, want {PostalCode:20811, Anchor:washington-dc-metro, Reason:hud:msa}", got[0])
	}
}

func TestCrosswalkHUDBackfill_SkipsZIPsAlreadyResolvedByZCTA(t *testing.T) {
	huds := []HUDZipCounty{
		{ZIP: "10001", County: "36061", TotRatio: 1.0},
	}
	zctaAnchors := []PostalAnchor{
		{PostalCode: "10001", AnchorSlug: "manhattan", Reason: "nyc-borough"},
	}
	countyToMSA := map[string]string{"36061": "35620"}
	msaSlugs := map[string]string{"35620": "nyc-metro"}

	got, _ := CrosswalkHUDBackfill(huds, zctaAnchors, countyToMSA, msaSlugs, nil)
	if len(got) != 0 {
		t.Errorf("HUD-backfill should skip ZIPs already in ZCTA output; got %+v", got)
	}
}

func TestCrosswalkHUDBackfill_20811_AnchorsToDCMetro(t *testing.T) {
	// Golden case from the slice spec. Real-world HUD row for ZIP
	// 20811 anchors to Montgomery County, MD (24031), which is in
	// CBSA 47900 (Washington-Arlington-Alexandria MSA → slug
	// washington-dc-metro).
	huds := []HUDZipCounty{
		{ZIP: "20811", County: "24031", ResRatio: 0.0, BusRatio: 1.0, OthRatio: 0.0, TotRatio: 1.0},
	}
	countyToMSA := map[string]string{"24031": "47900"}
	msaSlugs := map[string]string{"47900": "washington-dc-metro"}

	got, _ := CrosswalkHUDBackfill(huds, nil, countyToMSA, msaSlugs, nil)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %+v", len(got), got)
	}
	if got[0].AnchorSlug != "washington-dc-metro" {
		t.Errorf("anchor = %q, want washington-dc-metro", got[0].AnchorSlug)
	}
	if got[0].Reason != "hud:msa" {
		t.Errorf("reason = %q, want hud:msa", got[0].Reason)
	}
}

func TestCrosswalkHUDBackfill_NYCBoroughViaCountyFIPS(t *testing.T) {
	// HUD row with a Brooklyn county FIPS must anchor at the
	// brooklyn leaf via nycBoroughCounty, not at the MSA.
	huds := []HUDZipCounty{
		{ZIP: "11999", County: "36047", TotRatio: 1.0}, // Kings/Brooklyn
	}
	countyToMSA := map[string]string{"36047": "35620"}
	msaSlugs := map[string]string{"35620": "nyc-metro"}

	got, _ := CrosswalkHUDBackfill(huds, nil, countyToMSA, msaSlugs, nil)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %+v", len(got), got)
	}
	if got[0].AnchorSlug != "brooklyn" || got[0].Reason != "hud:nyc-borough" {
		t.Errorf("got %+v, want {Anchor:brooklyn, Reason:hud:nyc-borough}", got[0])
	}
}

func TestCrosswalkHUDBackfill_CountyLeafFallback(t *testing.T) {
	// Cook County (17031) is in countyToLeaf; HUD-anchored ZIPs land
	// at the curated cook-county leaf, not at the MSA.
	huds := []HUDZipCounty{
		{ZIP: "60999", County: "17031", TotRatio: 1.0},
	}
	countyToMSA := map[string]string{"17031": "16980"}
	msaSlugs := map[string]string{"16980": "chicago-metro"}

	got, _ := CrosswalkHUDBackfill(huds, nil, countyToMSA, msaSlugs, nil)
	if len(got) != 1 || got[0].AnchorSlug != "cook-county" || got[0].Reason != "hud:county-leaf" {
		t.Errorf("got %+v, want {Anchor:cook-county, Reason:hud:county-leaf}", got)
	}
}

func TestCrosswalkHUDBackfill_StateFallback(t *testing.T) {
	// County with no MSA + no curated leaf falls back to the state
	// via the 2-digit state FIPS prefix.
	huds := []HUDZipCounty{
		{ZIP: "82999", County: "56021", TotRatio: 1.0}, // WY, Laramie County
	}
	got, _ := CrosswalkHUDBackfill(huds, nil, map[string]string{}, map[string]string{}, nil)
	if len(got) != 1 || got[0].AnchorSlug != "wy" || got[0].Reason != "hud:state" {
		t.Errorf("got %+v, want {Anchor:wy, Reason:hud:state}", got)
	}
}

func TestCrosswalkHUDBackfill_UnknownCountyDropped(t *testing.T) {
	// 99999 isn't a real county FIPS — no MSA, no leaf, no state
	// (the two-digit prefix "99" isn't in stateFIPSToSlug). HUD-only
	// ZIPs that can't be placed are dropped from the anchor output
	// but counted in the returned reason map under "hud:unknown" so
	// operators see the drop rate in the orchestrator log.
	huds := []HUDZipCounty{
		{ZIP: "00000", County: "99999", TotRatio: 1.0},
	}
	got, reasons := CrosswalkHUDBackfill(huds, nil, map[string]string{}, map[string]string{}, nil)
	if len(got) != 0 {
		t.Errorf("unknown county should drop the ZIP; got %+v", got)
	}
	if reasons["hud:unknown"] != 1 {
		t.Errorf("hud:unknown count = %d, want 1; reasons = %+v", reasons["hud:unknown"], reasons)
	}
}

func TestCrosswalkHUDBackfill_OutputSortedByZIP(t *testing.T) {
	huds := []HUDZipCounty{
		{ZIP: "99999", County: "56021", TotRatio: 1.0}, // WY
		{ZIP: "20811", County: "24031", TotRatio: 1.0}, // MD → DC metro
		{ZIP: "60999", County: "17031", TotRatio: 1.0}, // cook-county
	}
	countyToMSA := map[string]string{"24031": "47900"}
	msaSlugs := map[string]string{"47900": "washington-dc-metro"}

	got, _ := CrosswalkHUDBackfill(huds, nil, countyToMSA, msaSlugs, nil)
	zips := make([]string, len(got))
	for i, a := range got {
		zips[i] = a.PostalCode
	}
	if !sort.StringsAreSorted(zips) {
		t.Errorf("output not sorted by ZIP: %v", zips)
	}
	want := []string{"20811", "60999", "99999"}
	if diff := cmp.Diff(want, zips); diff != "" {
		t.Errorf("sorted zips (-want +got):\n%s", diff)
	}
}
