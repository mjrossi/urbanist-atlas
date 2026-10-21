package etl

import (
	"strings"
	"unicode"
)

// Slugify lowercases s, folds common Latin diacritics, drops other
// punctuation, and joins words with single hyphens. It is the auto-slug
// generator for every country plan.
func Slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		r = unicode.ToLower(r)
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
	}
	out := b.String()
	return strings.TrimRight(out, "-")
}

// foldDiacritic maps lowercase Latin-1 Supplement and Latin Extended-A
// letters to their ASCII base; unlisted runes pass through and Slugify
// drops them. It covers the Spanish (PR CBSAs) and French (CA CMAs)
// that upstream titles use.
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
