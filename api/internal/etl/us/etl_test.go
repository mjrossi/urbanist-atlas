package us

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"New York", "new-york"},
		{"  Padding  Spaces  ", "padding-spaces"},
		{"Mayagüez", "mayaguez"},
		{"San José", "san-jose"},
		{"Wilkes-Barre--Hazleton", "wilkes-barre-hazleton"},
		{"Anchorage Municipality, AK", "anchorage-municipality-ak"},
		// All-punctuation collapses to empty — leftover defensive
		// behavior the caller depends on (autoSlug falls back to
		// "msa-<code>" when this returns "").
		{"!@#$%", ""},
		// Trailing separators trimmed.
		{"foo-", "foo"},
	}
	for _, c := range cases {
		got := slugify(c.in)
		if got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFoldDiacritic(t *testing.T) {
	// Spot-check one rune per Latin diacritic group. The full table
	// covers Latin-1 Supplement + Latin Extended-A, but exhaustive
	// testing would just retype the table — sample for coverage.
	cases := []struct {
		in   rune
		want rune
	}{
		{'á', 'a'},
		{'é', 'e'},
		{'í', 'i'},
		{'ñ', 'n'},
		{'ó', 'o'},
		{'ú', 'u'},
		{'ç', 'c'},
		{'ł', 'l'},
		{'ş', 's'},
		{'ž', 'z'},
		// Unmapped rune passes through.
		{'A', 'A'},
	}
	for _, c := range cases {
		if got := foldDiacritic(c.in); got != c.want {
			t.Errorf("foldDiacritic(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFirstCity(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"New York-Newark-Jersey City, NY-NJ-PA", "New York"},
		{"Boston-Cambridge-Newton, MA-NH", "Boston"},
		// Census uses double hyphen when a city itself contains a
		// hyphen. firstCity must still stop at the first separator
		// (it returns "Scranton", not "Scranton--Wilkes").
		{"Scranton--Wilkes-Barre, PA", "Scranton"},
		// Single-city, no hyphen.
		{"Anchorage, AK", "Anchorage"},
		// Defensive: missing comma.
		{"Somewhere With No Comma", "Somewhere With No Comma"},
	}
	for _, c := range cases {
		got := firstCity(c.in)
		if got != c.want {
			t.Errorf("firstCity(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseStateAbbrevs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"New York-Newark-Jersey City, NY-NJ-PA", []string{"NY", "NJ", "PA"}},
		{"Boston-Cambridge-Newton, MA-NH", []string{"MA", "NH"}},
		{"Washington-Arlington-Alexandria, DC-VA-MD-WV", []string{"DC", "VA", "MD", "WV"}},
		{"Anchorage, AK", []string{"AK"}},
		// PR puerto-rico — also 2-letter, also upper.
		{"San Juan-Bayamón-Caguas, PR", []string{"PR"}},
		// No comma — defensive nil.
		{"Free-Form Title", nil},
		// Lowercase wouldn't be an abbreviation — skipped.
		{"Foo, ny-NJ", []string{"NJ"}},
	}
	for _, c := range cases {
		got := parseStateAbbrevs(c.in)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("parseStateAbbrevs(%q) (-want +got):\n%s", c.in, diff)
		}
	}
}

func TestAutoSlugAndName(t *testing.T) {
	cases := []struct {
		msa      MSA
		wantSlug string
		wantName string
	}{
		{
			MSA{CBSACode: "47900", Title: "Washington-Arlington-Alexandria, DC-VA-MD-WV", StateAbbrevs: []string{"DC", "VA", "MD", "WV"}},
			"washington-dc-metro",
			"Washington Metro",
		},
		{
			MSA{CBSACode: "39580", Title: "Raleigh-Cary, NC", StateAbbrevs: []string{"NC"}},
			"raleigh-nc-metro",
			"Raleigh Metro",
		},
		{
			// Defensive: empty title → falls back to "msa-<code>".
			MSA{CBSACode: "99999", Title: "", StateAbbrevs: nil},
			"msa-99999",
			"",
		},
	}
	for _, c := range cases {
		if got := autoSlug(c.msa); got != c.wantSlug {
			t.Errorf("autoSlug(%s) = %q, want %q", c.msa.CBSACode, got, c.wantSlug)
		}
		if got := autoName(c.msa); got != c.wantName {
			t.Errorf("autoName(%s) = %q, want %q", c.msa.CBSACode, got, c.wantName)
		}
	}
}

func TestAutoParents(t *testing.T) {
	// Multi-state title produces all unique parents in title order.
	got := autoParents(MSA{Title: "x", StateAbbrevs: []string{"NY", "NJ", "PA"}})
	want := []string{"ny", "nj", "pa"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("autoParents NY-NJ-PA (-want +got):\n%s", diff)
	}
	// Duplicate-state title (shouldn't occur, but Crosswalk dedupes).
	got = autoParents(MSA{StateAbbrevs: []string{"NY", "NY"}})
	if diff := cmp.Diff([]string{"ny"}, got); diff != "" {
		t.Errorf("autoParents dedupe (-want +got):\n%s", diff)
	}
	// Disambiguated state slug (CA → ca-state).
	got = autoParents(MSA{StateAbbrevs: []string{"CA"}})
	if diff := cmp.Diff([]string{"ca-state"}, got); diff != "" {
		t.Errorf("autoParents CA suffix (-want +got):\n%s", diff)
	}
	// Unknown abbreviation — dropped.
	got = autoParents(MSA{StateAbbrevs: []string{"ZZ"}})
	if diff := cmp.Diff([]string{}, got); diff != "" {
		t.Errorf("autoParents unknown (-want +got):\n%s", diff)
	}
}

func TestAssignMSASlugs_OverrideWins(t *testing.T) {
	msas := []MSA{
		{CBSACode: "35620", Title: "New York-Newark-Jersey City, NY-NJ-PA", StateAbbrevs: []string{"NY", "NJ", "PA"}},
		{CBSACode: "47900", Title: "Washington-Arlington-Alexandria, DC-VA-MD-WV", StateAbbrevs: []string{"DC", "VA", "MD", "WV"}},
	}
	overrides := []MSAOverride{
		{CBSACode: "35620", Slug: "nyc-metro", Name: "New York Metro", Parents: []string{"nyc-tristate"}},
	}
	got := AssignMSASlugs(msas, overrides)

	if got["35620"].Slug != "nyc-metro" {
		t.Errorf("override slug = %q, want nyc-metro", got["35620"].Slug)
	}
	if diff := cmp.Diff([]string{"nyc-tristate"}, got["35620"].Parents); diff != "" {
		t.Errorf("override parents (-want +got):\n%s", diff)
	}
	// Non-overridden MSA falls through to auto-gen.
	if got["47900"].Slug != "washington-dc-metro" {
		t.Errorf("auto slug = %q, want washington-dc-metro", got["47900"].Slug)
	}
	if diff := cmp.Diff([]string{"dc", "va", "md", "wv"}, got["47900"].Parents); diff != "" {
		t.Errorf("auto parents (-want +got):\n%s", diff)
	}
}

func TestCrosswalk_ReasonPriority(t *testing.T) {
	// Fixture exercises every reason bucket. The crosswalk tries
	// city-leaf → nyc-borough → county-leaf → msa → state → unknown,
	// in that order. We seed one ZCTA per bucket plus one that should
	// short-circuit to "unknown".
	zctaPlace := map[string]ZCTAPlace{
		"10001": {PlaceGEOID: "3651000"},     // NYC city — not in placeToLeaf, falls through
		"02115": {PlaceGEOID: "2507000"},     // Boston city → city-leaf
		"60601": {PlaceGEOID: "1714000"},     // Chicago city → city-leaf (also Cook County)
		"60002": {PlaceGEOID: "1700000"},     // unknown place → fall through to county
		"99999": {PlaceGEOID: "0000000"},     // unknown both → unknown bucket
		"39580": {PlaceGEOID: "3754860"},     // Raleigh (not curated) → fall through to MSA
	}
	zctaCounty := map[string]ZCTACounty{
		"10001": {CountyGEOID: "36061"},      // Manhattan → nyc-borough
		"02115": {CountyGEOID: "25025"},      // Suffolk County, MA
		"60601": {CountyGEOID: "17031"},      // Cook County — but place-leaf wins
		"60002": {CountyGEOID: "17031"},      // Cook County → county-leaf
		"39580": {CountyGEOID: "37183"},      // Wake County, NC → MSA
		// 99999 has no county row.
		"82001": {CountyGEOID: "56021"},      // Cheyenne, WY → state fallback
	}
	countyToMSA := map[string]string{
		"25025": "14460", // Boston MSA
		"36061": "35620", // NYC MSA
		"17031": "16980", // Chicago MSA
		"37183": "39580", // Raleigh MSA
	}
	msaSlugs := map[string]string{
		"14460": "greater-boston",
		"35620": "nyc-metro",
		"16980": "chicago-metro",
		"39580": "raleigh-nc-metro",
	}

	anchors, reasons := Crosswalk(zctaPlace, zctaCounty, countyToMSA, msaSlugs)

	got := map[string]PostalAnchor{}
	for _, a := range anchors {
		got[a.ZCTA] = a
	}

	type want struct {
		slug, reason string
	}
	expectations := map[string]want{
		"02115": {"boston", "city-leaf"},
		"60601": {"chicago", "city-leaf"},
		"10001": {"manhattan", "nyc-borough"},
		"60002": {"cook-county", "county-leaf"},
		"39580": {"raleigh-nc-metro", "msa"},
		"82001": {"wy", "state"},
	}
	for zcta, w := range expectations {
		a, ok := got[zcta]
		if !ok {
			t.Errorf("ZCTA %s missing from anchors", zcta)
			continue
		}
		if a.AnchorSlug != w.slug || a.Reason != w.reason {
			t.Errorf("ZCTA %s = (%s,%s), want (%s,%s)", zcta, a.AnchorSlug, a.Reason, w.slug, w.reason)
		}
	}
	if _, ok := got["99999"]; ok {
		t.Errorf("ZCTA 99999 should have been dropped to unknown bucket")
	}
	if reasons["unknown"] != 1 {
		t.Errorf("unknown count = %d, want 1", reasons["unknown"])
	}
	// Output is sorted by ZCTA for deterministic CSV emission.
	for i := 1; i < len(anchors); i++ {
		if anchors[i-1].ZCTA > anchors[i].ZCTA {
			t.Errorf("anchors not sorted: %q before %q", anchors[i-1].ZCTA, anchors[i].ZCTA)
		}
	}
}

func TestParseStateAbbrevs_NoStateSuffix(t *testing.T) {
	// Defensive: a CBSA title that ends with the state but no separator
	// before it (e.g., "Aguadilla-Isabela, PR" — Puerto Rico, single
	// state) should still parse cleanly.
	got := parseStateAbbrevs("Aguadilla-Isabela, PR")
	if diff := cmp.Diff([]string{"PR"}, got); diff != "" {
		t.Errorf("parseStateAbbrevs PR (-want +got):\n%s", diff)
	}
}

func TestSlugify_DoesNotPropagateInteriorWhitespace(t *testing.T) {
	// Multiple consecutive separators collapse to one hyphen.
	in := "  Foo   Bar  -  Baz  "
	want := "foo-bar-baz"
	if got := slugify(in); got != want {
		t.Errorf("slugify(%q) = %q, want %q", in, got, want)
	}
	// Documentation: a stand-alone slugify never starts with a
	// hyphen (prevDash blocks the leading separator from emitting).
	if strings.HasPrefix(slugify(in), "-") {
		t.Errorf("slugify must not emit leading hyphen")
	}
}

func TestWritePostalCodesCSV_MergesAndDedupsWithZCTAWinning(t *testing.T) {
	// ZCTA pass produced two ZIPs (10001 → manhattan, 20002 →
	// washington-dc); HUD pass produced one new ZIP (20811 →
	// washington-dc-metro) and one duplicate of a ZCTA ZIP
	// (10001 → nyc-metro). The merge must emit three rows sorted
	// ASC by postal_code, with the ZCTA-source anchor for 10001
	// winning the tie against the HUD entry.
	zcta := []PostalAnchor{
		{ZCTA: "10001", AnchorSlug: "manhattan", Reason: "nyc-borough"},
		{ZCTA: "20002", AnchorSlug: "washington-dc", Reason: "city-leaf"},
	}
	hud := []PostalAnchor{
		{ZCTA: "20811", AnchorSlug: "washington-dc-metro", Reason: "hud:msa"},
		{ZCTA: "10001", AnchorSlug: "nyc-metro", Reason: "hud:msa"},
	}
	var buf strings.Builder
	if err := WritePostalCodesCSV(&buf, zcta, hud); err != nil {
		t.Fatalf("WritePostalCodesCSV: %v", err)
	}
	got := buf.String()
	want := "postal_code,country,leaf_region_slug\n" +
		"10001,US,manhattan\n" +
		"20002,US,washington-dc\n" +
		"20811,US,washington-dc-metro\n"
	if got != want {
		t.Errorf("CSV (-want +got):\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestWritePostalCodesCSV_NilHUDPreservesZCTAOnlyBehavior(t *testing.T) {
	// Pre-#7.5.5 callers (none post-merge, but defensive) can still
	// pass nil for the HUD slice and get ZCTA-only output sorted by
	// postal code.
	zcta := []PostalAnchor{
		{ZCTA: "20002", AnchorSlug: "washington-dc"},
		{ZCTA: "10001", AnchorSlug: "manhattan"},
	}
	var buf strings.Builder
	if err := WritePostalCodesCSV(&buf, zcta, nil); err != nil {
		t.Fatalf("WritePostalCodesCSV: %v", err)
	}
	got := buf.String()
	want := "postal_code,country,leaf_region_slug\n" +
		"10001,US,manhattan\n" +
		"20002,US,washington-dc\n"
	if got != want {
		t.Errorf("CSV (-want +got):\nwant:\n%s\ngot:\n%s", want, got)
	}
}
