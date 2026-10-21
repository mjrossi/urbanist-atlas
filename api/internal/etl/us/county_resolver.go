package us

// countyResolver maps a 5-digit county GEOID to the smallest curated
// region anchor for it, smallest first:
//
//  1. NYC borough leaf
//  2. curated non-NYC county leaf
//  3. the CBSA's own-state portion for a multi-state MSA, else the MSA
//  4. state / territory, by 2-digit FIPS prefix
//
// The ZCTA path consults it after the city-place tier fails; HUD's
// ZIP-County crosswalk carries no place GEOID, so for HUD the resolver
// is the whole fallback.
type countyResolver struct {
	countyToMSA  map[string]string // 5-digit county FIPS → CBSA code
	msaSlugs     map[string]string // CBSA code → MSA umbrella slug
	portionSlugs map[string]string // "CBSAcode:stateFIPS" → per-state portion slug
}

// newCountyResolver builds a resolver from the per-run CBSA tables. A
// nil portionSlugs disables multi-state portion routing.
func newCountyResolver(countyToMSA, msaSlugs, portionSlugs map[string]string) countyResolver {
	return countyResolver{countyToMSA: countyToMSA, msaSlugs: msaSlugs, portionSlugs: portionSlugs}
}

// Resolve walks the fallback for a 5-digit county GEOID. reason is the
// tier label ("nyc-borough", "county-leaf", "msa-portion", "msa",
// "state"). ok is false when no tier matches, e.g. APO/FPO ZIPs whose
// county FIPS starts with "99".
func (r countyResolver) Resolve(countyGEOID string) (slug, reason string, ok bool) {
	if s := nycBoroughCounty[countyGEOID]; s != "" {
		return s, "nyc-borough", true
	}
	if s := countyToLeaf[countyGEOID]; s != "" {
		return s, "county-leaf", true
	}
	if code := r.countyToMSA[countyGEOID]; code != "" {
		// A portion's ancestor walk reaches only its own state, so a
		// multi-state metro doesn't leak the other states' orgs.
		if len(countyGEOID) >= 2 {
			if p := r.portionSlugs[code+":"+countyGEOID[:2]]; p != "" {
				return p, "msa-portion", true
			}
		}
		if s := r.msaSlugs[code]; s != "" {
			return s, "msa", true
		}
	}
	if len(countyGEOID) >= 2 {
		if s := stateFIPSToSlug[countyGEOID[:2]]; s != "" {
			return s, "state", true
		}
	}
	return "", "", false
}
