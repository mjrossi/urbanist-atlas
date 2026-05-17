package atlas

import (
	"fmt"
	"strings"
)

// NormalizePostalCode applies per-country canonicalization rules:
// uppercase + whitespace stripped for every country, plus country-
// specific truncation (CA → FSA, UK → outward code). Unknown countries
// get the generic uppercase/strip pass.
//
// Used by every consumer that needs to compare or store a postal code
// canonically: MemStore, the Postgres adapter, the seed loader, the
// loadpostal CSV reader, and the HTTP handler when normalizing query
// params.
func NormalizePostalCode(country Country, raw string) string {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(raw), " ", ""))
	switch country {
	case CountryCA:
		if len(s) > 3 {
			return s[:3]
		}
		return s
	case "UK":
		// Outward code is everything before the inward 3-char block.
		// A full UK postcode without spaces is 5–7 chars; an outward-only
		// code is 2–4. Only strip if we have a full postcode.
		if len(s) > 4 {
			return s[:len(s)-3]
		}
		return s
	case "PT":
		// PT postal codes are 7-digit NNNN-NNN (e.g. 1100-001 in Lisboa).
		// Strip the hyphen so storage is 7 raw digits; lookups apply the
		// same normalization, so "1100-001" and "1100001" resolve
		// identically.
		return strings.ReplaceAll(s, "-", "")
	default:
		return s
	}
}

// ValidatePostalCode enforces per-country length + character-class rules.
// Returns nil for unknown countries (we don't fail closed — seed data
// for a new country lands before there's a chance to add a validator).
func ValidatePostalCode(country Country, code string) error {
	switch country {
	case CountryUS:
		if len(code) != 5 {
			return fmt.Errorf("US ZIP %q: want 5 digits, got %d", code, len(code))
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				return fmt.Errorf("US ZIP %q: non-digit character", code)
			}
		}
	case CountryCA:
		if len(code) != 3 {
			return fmt.Errorf("CA FSA %q: want 3 chars, got %d", code, len(code))
		}
		if !isLetter(code[0]) || !isDigit(code[1]) || !isLetter(code[2]) {
			return fmt.Errorf("CA FSA %q: must be letter-digit-letter", code)
		}
	case "DE", "FR", "MX":
		if len(code) != 5 {
			return fmt.Errorf("%s postal code %q: want 5 digits", country, code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				return fmt.Errorf("%s postal code %q: non-digit character", country, code)
			}
		}
	case "UK":
		if len(code) < 2 || len(code) > 4 {
			return fmt.Errorf("UK outward code %q: want 2–4 chars", code)
		}
	case "AU":
		if len(code) != 4 {
			return fmt.Errorf("AU postcode %q: want 4 digits", code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				return fmt.Errorf("AU postcode %q: non-digit character", code)
			}
		}
	case "PT":
		// PT codes are 7-digit after NormalizePostalCode strips the hyphen.
		if len(code) != 7 {
			return fmt.Errorf("PT postal code %q: want 7 digits (after stripping hyphen)", code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				return fmt.Errorf("PT postal code %q: non-digit character", code)
			}
		}
	}
	return nil
}

// postalKey is the internal cache key used by MemStore. Lowercase
// helper, not exported.
func postalKey(country Country, code string) string {
	return string(country) + ":" + NormalizePostalCode(country, code)
}

func isLetter(b byte) bool { return b >= 'A' && b <= 'Z' }
func isDigit(b byte) bool  { return b >= '0' && b <= '9' }
