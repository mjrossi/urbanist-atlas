package seedfiles

import (
	"strings"
	"testing"
)

func TestValidateOrgFields_Required(t *testing.T) {
	tests := []struct {
		name    string
		n, sd   string
		url, cu string
		tags    []string
		slugs   []string
		want    string
	}{
		{"missing name", "", "d", "https://x.org", "", nil, []string{"brooklyn"}, "name required"},
		{"missing short_desc", "n", "", "https://x.org", "", nil, []string{"brooklyn"}, "short_desc required"},
		{"missing website_url", "n", "d", "", "", nil, []string{"brooklyn"}, "website_url required"},
		{"no region_slugs", "n", "d", "https://x.org", "", nil, nil, "region_slugs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOrgFields(tc.n, tc.sd, tc.url, tc.cu, tc.tags, tc.slugs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestValidateOrgFields_URLShape(t *testing.T) {
	tests := []struct {
		name, websiteURL string
		want             string // substring required in the error
	}{
		{"javascript scheme", "javascript:alert(1)", "must use http"},
		{"ftp scheme", "ftp://example.org/file", "must use http"},
		{"no scheme", "example.org", "must use http"},
		{"missing host", "https://", "host"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOrgFields("n", "d", tc.websiteURL, "", nil, []string{"brooklyn"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestValidateOrgFields_ContactURL_AcceptsMailto(t *testing.T) {
	if err := ValidateOrgFields("n", "d", "https://x.org", "mailto:hi@x.org", nil, []string{"brooklyn"}); err != nil {
		t.Fatalf("mailto contact_url should be accepted, got %v", err)
	}
	// Empty mailto is not a contact URL.
	if err := ValidateOrgFields("n", "d", "https://x.org", "mailto:", nil, []string{"brooklyn"}); err == nil {
		t.Fatal("bare mailto: should be rejected")
	}
}

func TestValidateOrgFields_LengthCaps(t *testing.T) {
	tooLongName := strings.Repeat("a", MaxNameLen+1)
	if err := ValidateOrgFields(tooLongName, "d", "https://x.org", "", nil, []string{"brooklyn"}); err == nil {
		t.Fatal("oversize name should be rejected")
	}
	tooLongDesc := strings.Repeat("a", MaxShortDescLen+1)
	if err := ValidateOrgFields("n", tooLongDesc, "https://x.org", "", nil, []string{"brooklyn"}); err == nil {
		t.Fatal("oversize short_desc should be rejected")
	}
	tooLongURL := "https://x.org/" + strings.Repeat("a", MaxURLLen)
	if err := ValidateOrgFields("n", "d", tooLongURL, "", nil, []string{"brooklyn"}); err == nil {
		t.Fatal("oversize website_url should be rejected")
	}
}

func TestValidateOrgFields_HappyPath(t *testing.T) {
	if err := ValidateOrgFields(
		"Brooklyn Greenways", "desc",
		"https://example.org", "https://example.org/contact",
		[]string{"cycling", "grassroots"},
		[]string{"brooklyn-ny"},
	); err != nil {
		t.Fatalf("happy path: %v", err)
	}
}
