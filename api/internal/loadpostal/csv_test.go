package loadpostal

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestParseCSV_USHappyPath(t *testing.T) {
	input := strings.Join([]string{
		"postal_code,country,city_name,city_slug,county_name,county_slug,metro_name,metro_slug,state_name,state_slug",
		"11217,US,Brooklyn,brooklyn-ny,\"Kings County, NY\",kings-county-ny,New York Metro,nyc-metro,NY,ny",
		"10001,US,Manhattan,manhattan-ny,\"New York County, NY\",new-york-county-ny,New York Metro,nyc-metro,NY,ny",
	}, "\n") + "\n"

	rows, err := ParseCSV(strings.NewReader(input), atlas.CountryUS)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	want := []Row{
		{
			PostalCode: "11217", Country: atlas.CountryUS,
			City:   RegionRef{Name: "Brooklyn", Slug: "brooklyn-ny"},
			County: RegionRef{Name: "Kings County, NY", Slug: "kings-county-ny"},
			Metro:  RegionRef{Name: "New York Metro", Slug: "nyc-metro"},
			State:  RegionRef{Name: "NY", Slug: "ny"},
		},
		{
			PostalCode: "10001", Country: atlas.CountryUS,
			City:   RegionRef{Name: "Manhattan", Slug: "manhattan-ny"},
			County: RegionRef{Name: "New York County, NY", Slug: "new-york-county-ny"},
			Metro:  RegionRef{Name: "New York Metro", Slug: "nyc-metro"},
			State:  RegionRef{Name: "NY", Slug: "ny"},
		},
	}
	if diff := cmp.Diff(want, rows); diff != "" {
		t.Errorf("rows (-want +got):\n%s", diff)
	}
}

func TestParseCSV_CANormalization(t *testing.T) {
	// Mixed case + spaces + full 6-char postal must reduce to 3-char FSA.
	input := "postal_code,country,city_name,city_slug,county_name,county_slug,metro_name,metro_slug,state_name,state_slug\n" +
		"m5v 3a8,CA,Toronto,toronto-on,,,Toronto CMA,toronto-cma,Ontario,ontario\n"

	rows, err := ParseCSV(strings.NewReader(input), atlas.CountryCA)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: want 1, got %d", len(rows))
	}
	if rows[0].PostalCode != "M5V" {
		t.Errorf("normalized postal code: want M5V, got %q", rows[0].PostalCode)
	}
	if rows[0].County.Slug != "" || rows[0].County.Name != "" {
		t.Errorf("CA row should have empty county, got %+v", rows[0].County)
	}
}

func TestParseCSV_RejectsMismatchedCountry(t *testing.T) {
	input := "postal_code,country,city_name,city_slug,county_name,county_slug,metro_name,metro_slug,state_name,state_slug\n" +
		"11217,US,Brooklyn,brooklyn-ny,,,,,NY,ny\n"
	_, err := ParseCSV(strings.NewReader(input), atlas.CountryCA)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "country") {
		t.Errorf("err = %v; want country mismatch", err)
	}
}

func TestParseCSV_RejectsBadHeader(t *testing.T) {
	input := "zip,country,city,slug\n11217,US,Brooklyn,brooklyn-ny\n"
	_, err := ParseCSV(strings.NewReader(input), atlas.CountryUS)
	if err == nil {
		t.Fatal("want header error, got nil")
	}
}

func TestParseCSV_RejectsInvalidPostalCode(t *testing.T) {
	cases := map[string]struct {
		input   string
		country atlas.Country
		wantSub string
	}{
		"US too short": {
			input:   "postal_code,country,city_name,city_slug,county_name,county_slug,metro_name,metro_slug,state_name,state_slug\n123,US,X,x,,,,,NY,ny\n",
			country: atlas.CountryUS,
			wantSub: "want 5 digits",
		},
		"US non-digit": {
			input:   "postal_code,country,city_name,city_slug,county_name,county_slug,metro_name,metro_slug,state_name,state_slug\n1A234,US,X,x,,,,,NY,ny\n",
			country: atlas.CountryUS,
			wantSub: "non-digit",
		},
		"CA wrong shape": {
			input:   "postal_code,country,city_name,city_slug,county_name,county_slug,metro_name,metro_slug,state_name,state_slug\n12V,CA,T,t,,,,,Ontario,ontario\n",
			country: atlas.CountryCA,
			wantSub: "letter-digit-letter",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseCSV(strings.NewReader(tc.input), tc.country)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %v; want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestParseCSV_RejectsHalfRegionPair(t *testing.T) {
	// county_name set, county_slug empty.
	input := "postal_code,country,city_name,city_slug,county_name,county_slug,metro_name,metro_slug,state_name,state_slug\n" +
		"11217,US,Brooklyn,brooklyn-ny,Kings,,,,NY,ny\n"
	_, err := ParseCSV(strings.NewReader(input), atlas.CountryUS)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "both or neither") {
		t.Errorf("err = %v; want both-or-neither error", err)
	}
}

func TestParseCSV_RequiresState(t *testing.T) {
	input := "postal_code,country,city_name,city_slug,county_name,county_slug,metro_name,metro_slug,state_name,state_slug\n" +
		"11217,US,Brooklyn,brooklyn-ny,,,,,,\n"
	_, err := ParseCSV(strings.NewReader(input), atlas.CountryUS)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("err = %v; want required-state error", err)
	}
}

func TestScopeTierFor(t *testing.T) {
	cases := map[atlas.RegionKind]atlas.ScopeTier{
		atlas.RegionCity:       atlas.ScopeLocal,
		atlas.RegionCounty:     atlas.ScopeLocal,
		atlas.RegionMetro:      atlas.ScopeRegional,
		atlas.RegionState:      atlas.ScopeRegional,
		atlas.RegionProvince:   atlas.ScopeRegional,
		atlas.RegionCountry:    atlas.ScopeRegional,
		atlas.RegionMultiState: atlas.ScopeRegional,
	}
	for k, want := range cases {
		if got := scopeTierFor(k); got != want {
			t.Errorf("scopeTierFor(%q) = %q, want %q", k, got, want)
		}
	}
}

func TestNormalizePostalCode(t *testing.T) {
	cases := []struct {
		country atlas.Country
		in, out string
	}{
		{atlas.CountryUS, " 11217 ", "11217"},
		{atlas.CountryCA, "m5v 3a8", "M5V"},
		{atlas.CountryCA, "M5V", "M5V"},
		{atlas.CountryUS, "10001", "10001"},
	}
	for _, c := range cases {
		if got := normalizePostalCode(c.country, c.in); got != c.out {
			t.Errorf("normalizePostalCode(%q, %q) = %q, want %q", c.country, c.in, got, c.out)
		}
	}
}
