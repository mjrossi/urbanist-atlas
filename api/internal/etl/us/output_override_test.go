package us

import (
	"strings"
	"testing"
)

// TestWritePostalCodesCSVAppliesZipOverride verifies that the editorial
// per-ZIP overrides in zipAnchorOverride win over both the ZCTA and HUD
// crosswalk results, while non-overridden rows pass through untouched.
//
// This is the regression guard for the Stamford/Tri-State fix: the
// residential Fairfield ZIPs that county-vintage skew stranded at the
// bare `ct` state anchor must come out anchored at bridgeport-ct-metro,
// which parents under nyc-tristate (see regions_us_msa_overrides.toml).
func TestWritePostalCodesCSVAppliesZipOverride(t *testing.T) {
	// Sanity: the override set must include the canonical Stamford ZIP,
	// otherwise the rest of this test is vacuous.
	if got := zipAnchorOverride["06902"]; got != "bridgeport-ct-metro" {
		t.Fatalf("zipAnchorOverride[06902] = %q, want %q", got, "bridgeport-ct-metro")
	}

	zcta := []PostalAnchor{
		// Would resolve to the bare state anchor without the override.
		{ZCTA: "06902", AnchorSlug: "ct", Reason: "state"},
		// A control row that must pass through unchanged.
		{ZCTA: "90210", AnchorSlug: "greater-la", Reason: "msa"},
	}
	// A HUD-sourced anchor for an overridden ZIP must also lose to the
	// override (override is applied after the ZCTA∪HUD merge).
	hud := []PostalAnchor{
		{ZCTA: "06850", AnchorSlug: "ct", Reason: "hud:state"},
	}

	var sb strings.Builder
	if err := WritePostalCodesCSV(&sb, zcta, hud, zipAnchorOverride); err != nil {
		t.Fatalf("WritePostalCodesCSV: %v", err)
	}
	got := sb.String()

	wantRows := []string{
		"06850,US,bridgeport-ct-metro", // overridden HUD row
		"06902,US,bridgeport-ct-metro", // overridden ZCTA row
		"90210,US,greater-la",          // control, untouched
	}
	for _, row := range wantRows {
		if !strings.Contains(got, row+"\n") {
			t.Errorf("output missing row %q\n--- full output ---\n%s", row, got)
		}
	}

	// The overridden ZIPs must NOT appear with their pre-override anchor.
	for _, bad := range []string{"06902,US,ct", "06850,US,ct"} {
		if strings.Contains(got, bad+"\n") {
			t.Errorf("output still contains pre-override row %q", bad)
		}
	}
}

// TestZipAnchorOverrideShape guards the override map's invariants: every
// entry points at the bridgeport-ct-metro slug (the only anchor this
// editorial override targets today) and keys are 5-digit ZIPs.
func TestZipAnchorOverrideShape(t *testing.T) {
	if len(zipAnchorOverride) == 0 {
		t.Fatal("zipAnchorOverride is empty")
	}
	for zip, slug := range zipAnchorOverride {
		if len(zip) != 5 {
			t.Errorf("override key %q is not a 5-digit ZIP", zip)
		}
		if slug != "bridgeport-ct-metro" {
			t.Errorf("override[%s] = %q, want bridgeport-ct-metro", zip, slug)
		}
	}
}
