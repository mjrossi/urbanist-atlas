package us

// countyResolver maps a 5-digit county GEOID to the smallest curated
// region anchor available for it. It encapsulates the 4-tier county
// fallback chain shared by two callers:
//
//   - Crosswalk (ZCTA path) consults it AFTER the city-place tier
//     fails. Place GEOIDs aren't county-keyed, so the place lookup
//     stays in the caller and the resolver picks up at NYC borough.
//   - CrosswalkHUDBackfill (HUD path) calls it directly. HUD's
//     quarterly ZIP↔County crosswalk doesn't carry a place GEOID, so
//     the place tier is skipped entirely — the resolver IS the whole
//     fallback for the HUD path.
//
// Resolution order, smallest curated region first:
//
//  1. NYC borough leaf (county ∈ {36005, 36047, 36061, 36081, 36085})
//  2. Curated non-NYC county leaf (Cook County IL, Lake County IN)
//  3. MSA region via countyToMSA → msaSlugs
//  4. State / territory region via 2-digit FIPS prefix
//
// When no tier matches (rare: APO/FPO 999xx ZIPs whose county FIPS
// starts with "99", or a malformed sub-2-char county), Resolve returns
// ("", "", false). Callers translate that into a dropped anchor.
//
// Adding a new tier — e.g., a "transit district" region between
// county-leaf and MSA — is a single insertion in Resolve. Both the
// ZCTA and HUD callers pick it up automatically.
type countyResolver struct {
	countyToMSA map[string]string // 5-digit county FIPS → CBSA code
	msaSlugs    map[string]string // CBSA code → MSA region slug
}

// newCountyResolver builds a resolver from the per-run lookup tables.
// The borough / county-leaf / state-FIPS maps are package-level
// constants (see mappings.go) and don't need to be passed in.
func newCountyResolver(countyToMSA, msaSlugs map[string]string) countyResolver {
	return countyResolver{countyToMSA: countyToMSA, msaSlugs: msaSlugs}
}

// Resolve walks the 4-tier fallback for a 5-digit county GEOID.
// Reason is the bare bucket label ("nyc-borough", "county-leaf",
// "msa", "state"); the HUD caller prefixes "hud:" so a merged
// reason-count histogram can tell the two sources apart.
func (r countyResolver) Resolve(countyGEOID string) (slug, reason string, ok bool) {
	if s := nycBoroughCounty[countyGEOID]; s != "" {
		return s, "nyc-borough", true
	}
	if s := countyToLeaf[countyGEOID]; s != "" {
		return s, "county-leaf", true
	}
	if s := r.msaSlugs[r.countyToMSA[countyGEOID]]; s != "" {
		return s, "msa", true
	}
	if len(countyGEOID) >= 2 {
		if s := stateFIPSToSlug[countyGEOID[:2]]; s != "" {
			return s, "state", true
		}
	}
	return "", "", false
}
