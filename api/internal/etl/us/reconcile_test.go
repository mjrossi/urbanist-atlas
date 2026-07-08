package us

import (
	"testing"
)

// TestReconcileCTLegacyCounties_RepairsStrandedStateAnchors is the
// regression guard for the Connecticut county-vintage fix. A CT ZCTA
// keyed by a retired legacy county (09001 Fairfield) that fell through
// to the bare `ct` state anchor must be re-anchored at its metro using
// HUD's current planning-region county (09120 Greater Bridgeport →
// bridgeport-ct-metro).
func TestReconcileCTLegacyCounties_RepairsStrandedStateAnchors(t *testing.T) {
	anchors := []PostalAnchor{
		// Stranded CT ZIP: legacy Fairfield county, resolved to state.
		{PostalCode: "06902", AnchorSlug: "ct", Reason: "state"},
		// Control: a non-CT state anchor must not be touched.
		{PostalCode: "82001", AnchorSlug: "wy", Reason: "state"},
	}
	zctaCounty := map[string]zctaCounty{
		"06902": {CountyGEOID: "09001"}, // legacy Fairfield County
		"82001": {CountyGEOID: "56021"}, // Laramie County, WY
	}
	huds := []hudZipCounty{
		// HUD keys 06902 by the current planning-region FIPS.
		{ZIP: "06902", County: "09120", TotRatio: 1.0}, // Greater Bridgeport PR
	}
	countyToMSA := map[string]string{
		"09120": "14860", // Greater Bridgeport PR → Bridgeport-Stamford CBSA
	}
	msaSlugs := map[string]string{
		"14860": "bridgeport-ct-metro",
	}

	counts := reconcileCTLegacyCounties(anchors, zctaCounty, huds, countyToMSA, msaSlugs, nil)

	if got := anchors[0]; got.AnchorSlug != "bridgeport-ct-metro" || got.Reason != "ct-reconciled:msa" {
		t.Errorf("06902 = %+v, want {bridgeport-ct-metro, ct-reconciled:msa}", got)
	}
	if got := anchors[1]; got.AnchorSlug != "wy" || got.Reason != "state" {
		t.Errorf("82001 (control) = %+v, want unchanged {wy, state}", got)
	}
	if counts["ct-reconciled:msa"] != 1 {
		t.Errorf("ct-reconciled:msa = %d, want 1; counts = %+v", counts["ct-reconciled:msa"], counts)
	}
}

// TestReconcileCTLegacyCounties_LeavesFinerTiersAlone verifies the
// smallest-anchor invariant: a CT ZCTA that already won a finer tier
// (the bridgeport city-leaf via the place crosswalk) must NOT be
// downgraded to the metro, even though its legacy county is in scope.
func TestReconcileCTLegacyCounties_LeavesFinerTiersAlone(t *testing.T) {
	anchors := []PostalAnchor{
		{PostalCode: "06604", AnchorSlug: "bridgeport", Reason: "city-leaf"},
	}
	zctaCounty := map[string]zctaCounty{
		"06604": {CountyGEOID: "09001"}, // legacy Fairfield County
	}
	huds := []hudZipCounty{
		{ZIP: "06604", County: "09120", TotRatio: 1.0},
	}
	countyToMSA := map[string]string{"09120": "14860"}
	msaSlugs := map[string]string{"14860": "bridgeport-ct-metro"}

	reconcileCTLegacyCounties(anchors, zctaCounty, huds, countyToMSA, msaSlugs, nil)

	if got := anchors[0]; got.AnchorSlug != "bridgeport" || got.Reason != "city-leaf" {
		t.Errorf("06604 = %+v, want unchanged {bridgeport, city-leaf}", got)
	}
}

// TestReconcileCTLegacyCounties_RuralStaysAtState verifies that a CT ZIP
// whose current-vintage county also has no MSA stays at the state anchor
// (no spurious change), and is counted as unchanged rather than
// reconciled.
func TestReconcileCTLegacyCounties_RuralStaysAtState(t *testing.T) {
	anchors := []PostalAnchor{
		{PostalCode: "06750", AnchorSlug: "ct", Reason: "state"},
	}
	zctaCounty := map[string]zctaCounty{
		"06750": {CountyGEOID: "09005"}, // legacy Litchfield County
	}
	huds := []hudZipCounty{
		{ZIP: "06750", County: "09160", TotRatio: 1.0}, // Northwest Hills PR
	}
	// 09160 has no MSA entry → countyResolver falls back to ct via FIPS.
	countyToMSA := map[string]string{}
	msaSlugs := map[string]string{}

	counts := reconcileCTLegacyCounties(anchors, zctaCounty, huds, countyToMSA, msaSlugs, nil)

	if got := anchors[0]; got.AnchorSlug != "ct" || got.Reason != "state" {
		t.Errorf("06750 = %+v, want unchanged {ct, state}", got)
	}
	if counts["ct-unchanged:state"] != 1 {
		t.Errorf("ct-unchanged:state = %d, want 1; counts = %+v", counts["ct-unchanged:state"], counts)
	}
}

// TestReconcileCTLegacyCounties_HUDUnresolvedSkipped covers the branch
// where HUD's primary county can't be resolved at all (e.g. an APO/FPO
// 999xx FIPS whose 2-digit prefix isn't a state). The anchor must stay
// at state and the skip must be counted.
func TestReconcileCTLegacyCounties_HUDUnresolvedSkipped(t *testing.T) {
	anchors := []PostalAnchor{
		{PostalCode: "06902", AnchorSlug: "ct", Reason: "state"},
	}
	zctaCounty := map[string]zctaCounty{
		"06902": {CountyGEOID: "09001"}, // legacy Fairfield County
	}
	huds := []hudZipCounty{
		{ZIP: "06902", County: "99999", TotRatio: 1.0}, // unresolvable FIPS
	}

	counts := reconcileCTLegacyCounties(anchors, zctaCounty, huds, map[string]string{}, map[string]string{}, nil)

	if got := anchors[0]; got.AnchorSlug != "ct" || got.Reason != "state" {
		t.Errorf("06902 = %+v, want unchanged {ct, state}", got)
	}
	if counts["ct-skip:hud-unresolved"] != 1 {
		t.Errorf("ct-skip:hud-unresolved = %d, want 1; counts = %+v", counts["ct-skip:hud-unresolved"], counts)
	}
}

// TestReconcileCTLegacyCounties_NonLegacyAndNoHUDAndEmpty covers the
// guard rails: a current-FIPS CT ZCTA is skipped (not a legacy county),
// a legacy ZIP absent from HUD is counted as a skip, and an empty HUD
// slice makes the whole pass a no-op.
func TestReconcileCTLegacyCounties_NonLegacyAndNoHUDAndEmpty(t *testing.T) {
	countyToMSA := map[string]string{"09120": "14860"}
	msaSlugs := map[string]string{"14860": "bridgeport-ct-metro"}

	// (a) Already keyed by a current planning region — not a legacy
	// county, so reconcile must not touch it even though it's at state.
	t.Run("non-legacy county skipped", func(t *testing.T) {
		anchors := []PostalAnchor{{PostalCode: "06000", AnchorSlug: "ct", Reason: "state"}}
		zctaCounty := map[string]zctaCounty{"06000": {CountyGEOID: "09120"}}
		huds := []hudZipCounty{{ZIP: "06000", County: "09120", TotRatio: 1.0}}
		counts := reconcileCTLegacyCounties(anchors, zctaCounty, huds, countyToMSA, msaSlugs, nil)
		if anchors[0].Reason != "state" {
			t.Errorf("non-legacy county should be untouched, got %+v", anchors[0])
		}
		if len(counts) != 0 {
			t.Errorf("expected no counts for non-legacy skip, got %+v", counts)
		}
	})

	// (b) Legacy county but ZIP not present in HUD → counted as skip.
	t.Run("legacy zip absent from HUD", func(t *testing.T) {
		anchors := []PostalAnchor{{PostalCode: "06902", AnchorSlug: "ct", Reason: "state"}}
		zctaCounty := map[string]zctaCounty{"06902": {CountyGEOID: "09001"}}
		huds := []hudZipCounty{{ZIP: "07030", County: "34017", TotRatio: 1.0}} // unrelated ZIP
		counts := reconcileCTLegacyCounties(anchors, zctaCounty, huds, countyToMSA, msaSlugs, nil)
		if anchors[0].Reason != "state" {
			t.Errorf("ZIP absent from HUD should stay at state, got %+v", anchors[0])
		}
		if counts["ct-skip:no-hud"] != 1 {
			t.Errorf("ct-skip:no-hud = %d, want 1; counts = %+v", counts["ct-skip:no-hud"], counts)
		}
	})

	// (c) Empty HUD slice → no-op, empty counts.
	t.Run("empty HUD is a no-op", func(t *testing.T) {
		anchors := []PostalAnchor{{PostalCode: "06902", AnchorSlug: "ct", Reason: "state"}}
		zctaCounty := map[string]zctaCounty{"06902": {CountyGEOID: "09001"}}
		counts := reconcileCTLegacyCounties(anchors, zctaCounty, nil, countyToMSA, msaSlugs, nil)
		if anchors[0].Reason != "state" || len(counts) != 0 {
			t.Errorf("empty HUD must be a no-op; anchor=%+v counts=%+v", anchors[0], counts)
		}
	})
}

// TestHUDPrimaryCounty_PicksMaxTotRatio guards the shared county-pick
// helper used by both the backfill and the CT reconcile.
func TestHUDPrimaryCounty_PicksMaxTotRatio(t *testing.T) {
	huds := []hudZipCounty{
		{ZIP: "20811", County: "24033", TotRatio: 0.10},
		{ZIP: "20811", County: "24031", TotRatio: 0.90}, // wins
		{ZIP: "06902", County: "09120", TotRatio: 1.0},
	}
	got := hudPrimaryCounty(huds)
	if got["20811"] != "24031" {
		t.Errorf("20811 primary county = %q, want 24031", got["20811"])
	}
	if got["06902"] != "09120" {
		t.Errorf("06902 primary county = %q, want 09120", got["06902"])
	}
}
