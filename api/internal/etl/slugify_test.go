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

// TestSlugify_FoldsLatinExtendedASuperset locks in the C1 consolidation
// decision: the shared fold table covers the full Latin-1 Supplement +
// Latin Extended-A range, not just Latin-1. Every accented letter below
// is Latin Extended-A (U+0100–U+017F) — characters the pre-consolidation
// CA slugifier folded only over Latin-1 and so silently *dropped*. With a
// narrowed table "Łódź" would slug to "ld" (the diacritic letters fall
// through as non-[a-z] runes and get removed), not "lodz". These cases
// fail loudly if a future change re-narrows the range, guarding the
// superset choice end-to-end through Slugify rather than retyping the
// table (which TestFoldDiacritic already samples).
func TestSlugify_FoldsLatinExtendedASuperset(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Łódź", "lodz"},       // ł, ź — Extended-A; ó is Latin-1
		{"Gdańsk", "gdansk"},   // ń — Extended-A
		{"Plzeň", "plzen"},     // ň — Extended-A
		{"Ōtsu", "otsu"},       // ō — Extended-A macron (uppercase folds via ToLower)
		{"Kraśnik", "krasnik"}, // ś — Extended-A
	}
	for _, c := range cases {
		if got := Slugify(c.in); got != c.want {
			t.Errorf("Slugify(%q) = %q, want %q (Latin Extended-A must fold, not drop)", c.in, got, c.want)
		}
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
		{'á', 'a'},
		{'â', 'a'},
		{'é', 'e'},
		{'è', 'e'},
		{'ê', 'e'},
		{'í', 'i'},
		{'î', 'i'},
		{'ñ', 'n'},
		{'ó', 'o'},
		{'ô', 'o'},
		{'ö', 'o'},
		{'ú', 'u'},
		{'û', 'u'},
		{'ù', 'u'},
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
