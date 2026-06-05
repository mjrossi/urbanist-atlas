package etl

import (
	"strings"
	"unicode"
)

// Slugify lowercases, drops diacritics, removes punctuation, and
// collapses whitespace + interior hyphens/underscores/slashes into
// single hyphens, trimming any trailing separator. It is the shared
// slug generator for every country plan's region auto-slug path (US
// MSA first-cities, CA CMA names). The fold table (foldDiacritic)
// covers the Latin-1 Supplement + Latin Extended-A — a superset of the
// few diacritics that actually appear in upstream titles (Spanish in
// PR CBSA rows, French in CA CMA names), so a new country reusing it
// gets correct folding without retabulating.
func Slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		r = unicode.ToLower(r)
		// Fold common Latin diacritics: ü→u, é→e, etc. The diacritic
		// is dropped; the base letter is kept.
		if r >= 0x00C0 && r <= 0x017F {
			r = foldDiacritic(r)
		}
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
		// All other runes (punctuation, etc.) are dropped silently.
	}
	out := b.String()
	return strings.TrimRight(out, "-")
}

// foldDiacritic maps the Latin-1 Supplement + Latin Extended-A range
// to ASCII letters. Slugify lowercases before calling this, so only the
// lowercase forms need cases here. Coverage is good enough for the
// diacritics that appear in upstream titles (Spanish in PR CBSA rows,
// French in CA CMA names).
func foldDiacritic(r rune) rune {
	switch r {
	case 'à', 'á', 'â', 'ã', 'ä', 'å', 'ā', 'ă', 'ą':
		return 'a'
	case 'è', 'é', 'ê', 'ë', 'ē', 'ĕ', 'ė', 'ę', 'ě':
		return 'e'
	case 'ì', 'í', 'î', 'ï', 'ĩ', 'ī', 'ĭ', 'į':
		return 'i'
	case 'ñ', 'ń', 'ņ', 'ň':
		return 'n'
	case 'ò', 'ó', 'ô', 'õ', 'ö', 'ø', 'ō', 'ŏ', 'ő':
		return 'o'
	case 'ù', 'ú', 'û', 'ü', 'ũ', 'ū', 'ŭ', 'ů', 'ű', 'ų':
		return 'u'
	case 'ÿ', 'ý':
		return 'y'
	case 'ç', 'ć', 'ĉ', 'ċ', 'č':
		return 'c'
	case 'ł', 'ļ', 'ľ':
		return 'l'
	case 'ś', 'ŝ', 'ş', 'š':
		return 's'
	case 'ź', 'ż', 'ž':
		return 'z'
	}
	return r
}
