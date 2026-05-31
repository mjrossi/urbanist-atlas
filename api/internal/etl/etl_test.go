package etl

import "testing"

func TestTargetMembership(t *testing.T) {
	cases := []struct {
		target          Target
		regions, postal bool
	}{
		{TargetAll, true, true},
		{TargetRegions, true, false},
		{TargetPostal, false, true},
	}
	for _, c := range cases {
		if got := c.target.Regions(); got != c.regions {
			t.Errorf("%q.Regions() = %v, want %v", c.target, got, c.regions)
		}
		if got := c.target.Postal(); got != c.postal {
			t.Errorf("%q.Postal() = %v, want %v", c.target, got, c.postal)
		}
	}
}

func TestParseTarget(t *testing.T) {
	for _, in := range []string{"all", "regions", "postal"} {
		if _, err := ParseTarget(in); err != nil {
			t.Errorf("ParseTarget(%q) unexpected err: %v", in, err)
		}
	}
	if _, err := ParseTarget("bogus"); err == nil {
		t.Error("ParseTarget(\"bogus\") = nil err, want error")
	}
}

func TestSourceFeedsTarget(t *testing.T) {
	all := SourceDescriptor{Filename: "a"} // empty Targets ⇒ feeds everything
	postalOnly := SourceDescriptor{Filename: "h", Targets: []Target{TargetPostal}}
	if !all.FeedsTarget(TargetRegions) || !all.FeedsTarget(TargetPostal) {
		t.Error("empty Targets should feed every target")
	}
	if postalOnly.FeedsTarget(TargetRegions) {
		t.Error("postal-only source should not feed regions")
	}
	if !postalOnly.FeedsTarget(TargetPostal) || !postalOnly.FeedsTarget(TargetAll) {
		t.Error("postal-only source should feed postal and all")
	}
}
