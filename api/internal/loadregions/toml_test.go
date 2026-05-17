package loadregions

import (
	"strings"
	"testing"
)

func TestParse_Minimal(t *testing.T) {
	src := `
[[region]]
slug = "ny"
kind = "us:state"
name = "New York"
scope_tier = "regional"
sort_priority = 60
parents = []

[[region]]
slug = "brooklyn"
kind = "us:borough"
name = "Brooklyn"
scope_tier = "local"
sort_priority = 10
parents = ["nyc"]
`
	f, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Regions) != 2 {
		t.Fatalf("want 2 regions, got %d", len(f.Regions))
	}
	if f.Regions[1].Slug != "brooklyn" || f.Regions[1].SortPriority != 10 {
		t.Errorf("brooklyn region: %+v", f.Regions[1])
	}
	if got := f.Regions[1].Parents; len(got) != 1 || got[0] != "nyc" {
		t.Errorf("brooklyn parents: %v", got)
	}
}

func TestParse_RejectsUnknownField(t *testing.T) {
	src := `
[[region]]
slug = "ny"
kind = "us:state"
name = "New York"
scope_tier = "regional"
sort_priority = 60
parents = []
mystery_field = "boom"
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestParse_RejectsInvalidScopeTier(t *testing.T) {
	src := `
[[region]]
slug = "ny"
kind = "us:state"
name = "New York"
scope_tier = "global"
sort_priority = 60
parents = []
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error for invalid scope_tier")
	}
}

func TestParse_RejectsEmpty(t *testing.T) {
	_, err := Parse(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}
