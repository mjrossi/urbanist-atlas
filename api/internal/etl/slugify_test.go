package etl

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// US MSA first-city fixtures.
		{"New York", "new-york"},
		{"  Padding  Spaces  ", "padding-spaces"},
		{"Mayagüez", "mayaguez"},
		{"San José", "san-jose"},
		{"Wilkes-Barre--Hazleton", "wilkes-barre-hazleton"},
		{"Anchorage Municipality, AK", "anchorage-municipality-ak"},
		// CA CMA-name fixtures.
		{"Toronto", "toronto"},
		{"Montréal", "montreal"},
		{"Trois-Rivières", "trois-rivieres"},
		{"Québec", "quebec"},
		{"Ottawa - Gatineau", "ottawa-gatineau"},
		{"   leading/trailing///   ", "leading-trailing"},
		// Underscores treated as separators too.
		{"foo_bar", "foo-bar"},
		// Punctuation dropped silently (no separator emitted).
		{"O'Brien", "obrien"},
		// All-punctuation collapses to empty — leftover defensive
		// behavior the caller depends on (US autoSlug falls back to
		// "msa-<code>" when this returns "").
		{"!@#$%", ""},
		// Trailing separators trimmed; empty input.
		{"foo-", "foo"},
		{"", ""},
	}
	for _, c := range cases {
		got := Slugify(c.in)
		if got != c.want {
			t.Errorf("Slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSlugify_DoesNotPropagateInteriorWhitespace(t *testing.T) {
	// Multiple consecutive separators collapse to one hyphen.
	in := "  Foo   Bar  -  Baz  "
	want := "foo-bar-baz"
	if got := Slugify(in); got != want {
		t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
	}
	// A stand-alone slugify never starts with a hyphen (prevDash blocks
	// the leading separator from emitting).
	if strings.HasPrefix(Slugify(in), "-") {
		t.Errorf("Slugify must not emit leading hyphen")
	}
}

func TestFoldDiacritic(t *testing.T) {
	// Spot-check one rune per Latin diacritic group. The full table
	// covers Latin-1 Supplement + Latin Extended-A, but exhaustive
	// testing would just retype the table — sample for coverage. Slugify
	// lowercases before folding, so only lowercase forms are tested.
	cases := []struct {
		in, want rune
	}{
		{'á', 'a'}, {'â', 'a'},
		{'é', 'e'}, {'è', 'e'}, {'ê', 'e'},
		{'í', 'i'}, {'î', 'i'},
		{'ñ', 'n'},
		{'ó', 'o'}, {'ô', 'o'}, {'ö', 'o'},
		{'ú', 'u'}, {'û', 'u'}, {'ù', 'u'},
		{'ç', 'c'},
		{'ł', 'l'},
		{'ş', 's'},
		{'ž', 'z'},
		// Unmapped runes pass through.
		{'A', 'A'},
		{'z', 'z'},
	}
	for _, c := range cases {
		if got := foldDiacritic(c.in); got != c.want {
			t.Errorf("foldDiacritic(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
