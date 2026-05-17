package atlas

import "testing"

func TestNormalizePostalCode(t *testing.T) {
	cases := []struct {
		name    string
		country Country
		in      string
		want    string
	}{
		{"us five digit", "US", "11217", "11217"},
		{"us with whitespace", "US", " 11217 ", "11217"},
		{"us strips inner spaces", "US", "11 217", "11217"},
		{"ca FSA upper", "CA", "M5V", "M5V"},
		{"ca FSA lower", "CA", "m5v", "M5V"},
		{"ca full code truncated to FSA", "CA", "M5V 3A8", "M5V"},
		{"ca lower full truncated", "CA", "m5v3a8", "M5V"},
		{"de five digit", "DE", "10115", "10115"},
		{"de with whitespace", "DE", " 10115 ", "10115"},
		{"fr five digit", "FR", "75001", "75001"},
		{"uk outward only when given inward", "UK", "SW1A 1AA", "SW1A"},
		{"uk outward already", "UK", "SW1A", "SW1A"},
		{"uk lower coerced", "UK", "sw1a 1aa", "SW1A"},
		{"au four digit", "AU", "2000", "2000"},
		{"mx five digit", "MX", "06600", "06600"},
		{"unknown country passthrough", "ZZ", "abc123", "ABC123"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizePostalCode(c.country, c.in)
			if got != c.want {
				t.Errorf("NormalizePostalCode(%q, %q) = %q, want %q", c.country, c.in, got, c.want)
			}
		})
	}
}

func TestValidatePostalCode(t *testing.T) {
	cases := []struct {
		name    string
		country Country
		in      string
		wantErr bool
	}{
		{"us valid", "US", "11217", false},
		{"us four digit", "US", "1121", true},
		{"us six digit", "US", "112170", true},
		{"us non-digit", "US", "11A17", true},
		{"ca valid FSA", "CA", "M5V", false},
		{"ca wrong shape (digit first)", "CA", "5MV", true},
		{"ca four char", "CA", "M5V1", true},
		{"de valid", "DE", "10115", false},
		{"de four digit", "DE", "1011", true},
		{"uk valid outward", "UK", "SW1A", false},
		{"uk too short", "UK", "S", true},
		{"au valid", "AU", "2000", false},
		{"au three digit", "AU", "200", true},
		{"unknown country passes", "ZZ", "anything", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePostalCode(c.country, c.in)
			if (err != nil) != c.wantErr {
				t.Errorf("ValidatePostalCode(%q, %q) err=%v, wantErr=%v", c.country, c.in, err, c.wantErr)
			}
		})
	}
}

func TestPostalKey(t *testing.T) {
	if got := postalKey("US", " 11217 "); got != "US:11217" {
		t.Errorf("postalKey US: got %q", got)
	}
	if got := postalKey("CA", "m5v 3a8"); got != "CA:M5V" {
		t.Errorf("postalKey CA: got %q", got)
	}
}
