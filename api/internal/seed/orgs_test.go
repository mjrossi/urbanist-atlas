package seed

import (
	"strings"
	"testing"
)

func TestParse_HappyPath(t *testing.T) {
	input := `orgs:
  - slug: transalt
    name: Transportation Alternatives
    short_desc: Streets and mobility advocacy.
    website_url: https://transalt.org
    contact_url: https://transalt.org/contact
    tags: [advocacy, safe-streets]
    regions:
      - country: US
        postal_codes: [11217, 10001]
`
	f, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Orgs) != 1 {
		t.Fatalf("orgs: want 1, got %d", len(f.Orgs))
	}
	o := f.Orgs[0]
	if o.Slug != "transalt" || o.Name != "Transportation Alternatives" {
		t.Errorf("org top-level: %+v", o)
	}
	if len(o.Regions) != 1 || o.Regions[0].Country != "US" || len(o.Regions[0].PostalCodes) != 2 {
		t.Errorf("org regions: %+v", o.Regions)
	}
}

func TestParse_RejectsDuplicateSlug(t *testing.T) {
	input := `orgs:
  - slug: a
    name: A
    short_desc: x
    website_url: https://a.example
    tags: [t]
    regions:
      - country: US
        postal_codes: [11217]
  - slug: a
    name: B
    short_desc: y
    website_url: https://b.example
    tags: [t]
    regions:
      - country: US
        postal_codes: [10001]
`
	_, err := Parse(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "duplicate slug") {
		t.Fatalf("want duplicate slug error, got %v", err)
	}
}

func TestParse_RejectsMissingFields(t *testing.T) {
	cases := map[string]string{
		"missing slug": `orgs:
  - name: A
    short_desc: x
    website_url: https://a.example
    tags: [t]
    regions:
      - country: US
        postal_codes: [11217]
`,
		"missing name": `orgs:
  - slug: a
    short_desc: x
    website_url: https://a.example
    tags: [t]
    regions:
      - country: US
        postal_codes: [11217]
`,
		"missing short_desc": `orgs:
  - slug: a
    name: A
    website_url: https://a.example
    tags: [t]
    regions:
      - country: US
        postal_codes: [11217]
`,
		"missing website_url": `orgs:
  - slug: a
    name: A
    short_desc: x
    tags: [t]
    regions:
      - country: US
        postal_codes: [11217]
`,
		"no regions": `orgs:
  - slug: a
    name: A
    short_desc: x
    website_url: https://a.example
    tags: [t]
`,
		"empty postal_codes": `orgs:
  - slug: a
    name: A
    short_desc: x
    website_url: https://a.example
    tags: [t]
    regions:
      - country: US
        postal_codes: []
`,
		"bad country": `orgs:
  - slug: a
    name: A
    short_desc: x
    website_url: https://a.example
    tags: [t]
    regions:
      - country: UK
        postal_codes: [SW1A]
`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(input))
			if err == nil {
				t.Fatalf("want error, got nil")
			}
		})
	}
}

func TestParse_RejectsEmptyFile(t *testing.T) {
	_, err := Parse(strings.NewReader("orgs: []\n"))
	if err == nil || !strings.Contains(err.Error(), "no orgs") {
		t.Fatalf("want no-orgs error, got %v", err)
	}
}

func TestParse_RejectsUnknownFields(t *testing.T) {
	input := `orgs:
  - slug: a
    name: A
    short_desc: x
    website_url: https://a.example
    tags: [t]
    bogus_field: hello
    regions:
      - country: US
        postal_codes: [11217]
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("want error on unknown field, got nil")
	}
}

func TestParse_BundledFixtureFileIsValid(t *testing.T) {
	// The committed orgs.yaml is the canonical seed; if it can't
	// parse, the binary can't seed.
	f, err := openTestFile(t)
	if err != nil {
		t.Skipf("seed fixture not reachable from test cwd: %v", err)
	}
	defer func() { _ = f.Close() }()
	parsed, err := Parse(f)
	if err != nil {
		t.Fatalf("bundled orgs.yaml failed to parse: %v", err)
	}
	if len(parsed.Orgs) < 10 {
		t.Errorf("expected at least 10 curated orgs, got %d", len(parsed.Orgs))
	}
}
