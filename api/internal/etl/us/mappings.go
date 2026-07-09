package us

// placeToLeaf maps Census place GEOIDs (state FIPS + place FIPS, 7
// digits) to the curated city-leaf slugs in api/seed/regions_us.toml.
//
// When you add a new curated city leaf to regions_us.toml, look up the
// place GEOID from the ZCTA-to-place crosswalk (a ZIP in that city →
// GEOID_PLACE_20 column) and add an entry here so future ETL runs
// anchor that city's ZIPs at the leaf.
//
// The crosswalk uses Census's "primary place" for each ZCTA (the place
// with the largest land-area overlap, AREALAND_PART). A ZCTA whose
// primary place is *not* one of the cities below — even if the ZIP
// physically overlaps that city — won't anchor at that leaf. Example:
// ZIP 07302 covers parts of Hoboken but its primary place is Jersey
// City, so it anchors at nyc-metro, not at hoboken. Adding an entry
// here only catches ZCTAs whose primary place matches.
//
// "New York city" (3651000) is intentionally NOT mapped here. NYC ZIPs
// anchor at borough leaves via nycBoroughCounty below — boroughs are
// counties, so the ZCTA-to-county crosswalk distinguishes them
// naturally where the place crosswalk can't.
var placeToLeaf = map[string]string{
	"0667000": "sf",            // San Francisco city, CA
	"2511000": "cambridge-ma",  // Cambridge city, MA
	"2507000": "boston",        // Boston city, MA
	"1714000": "chicago",       // Chicago city, IL
	"1754885": "oak-park",      // Oak Park village, IL
	"1827000": "gary",          // Gary city, IN
	"1245000": "miami",         // Miami city, FL
	"5363000": "seattle",       // Seattle city, WA
	"0644000": "los-angeles",   // Los Angeles city, CA
	"0908000": "bridgeport",    // Bridgeport city, CT
	"3432250": "hoboken",       // Hoboken city, NJ
	"1150000": "washington-dc", // Washington city, DC
}

// nycBoroughCounty maps the 5 NYC borough county GEOIDs (state FIPS +
// county FIPS, 5 digits) to borough-leaf slugs in regions_us.toml.
// This is the only place a county-level lookup short-circuits the
// city-place crosswalk — NYC is the lone US city we split
// sub-municipally at v1 (see docs/region-graph.md §8).
var nycBoroughCounty = map[string]string{
	"36005": "bronx",         // Bronx County, NY
	"36047": "brooklyn",      // Kings County, NY
	"36061": "manhattan",     // New York County, NY
	"36081": "queens",        // Queens County, NY
	"36085": "staten-island", // Richmond County, NY
}

// countyToLeaf maps non-NYC county GEOIDs to curated county leaves in
// regions_us.toml. Used as a fallback when no curated city-place leaf
// matches a ZCTA; consulted before the MSA tier, so a ZIP in one of
// these counties anchors at the county leaf rather than at the metro.
//
// The six Illinois entries are the RTA service area (Cook + the five
// collar counties). Seeding them as leaves is what lets collar-county
// suburbs reach Illinois statewide orgs through `rta-service-area → il`
// instead of through the metro — per docs/region-graph.md §1, a
// multi-state metro (Chicago spans IL+IN) must not carry a state edge,
// so the state reach lives on the county leaf's own ancestry.
var countyToLeaf = map[string]string{
	"17031": "cook-county",    // Cook County, IL (contains Chicago)
	"17043": "dupage-county",  // DuPage County, IL (RTA collar)
	"17089": "kane-county",    // Kane County, IL (RTA collar)
	"17097": "lake-county-il", // Lake County, IL (RTA collar)
	"17111": "mchenry-county", // McHenry County, IL (RTA collar)
	"17197": "will-county",    // Will County, IL (RTA collar)
	"18089": "lake-county-in", // Lake County, IN (contains Gary)
}

// stateFIPSToSlug maps 2-digit Census state FIPS codes to the state
// region slugs in api/seed/regions_us_states.toml. Includes all 50
// states + DC + Puerto Rico.
//
// Three slugs are suffix-disambiguated to avoid collisions with ISO
// country codes used as Country values on the wire:
//   - CA (FIPS 06): "ca-state" (vs Canada the country)
//   - DE (FIPS 10): "de-state" (vs Germany the country)
//   - LA (FIPS 22): "la-state" (vs the LA metro / Los Angeles)
var stateFIPSToSlug = map[string]string{
	"01": "al", "02": "ak", "04": "az", "05": "ar", "06": "ca-state",
	"08": "co", "09": "ct", "10": "de-state", "11": "dc", "12": "fl",
	"13": "ga", "15": "hi", "16": "id", "17": "il", "18": "in",
	"19": "ia", "20": "ks", "21": "ky", "22": "la-state", "23": "me",
	"24": "md", "25": "ma", "26": "mi", "27": "mn", "28": "ms",
	"29": "mo", "30": "mt", "31": "ne", "32": "nv", "33": "nh",
	"34": "nj", "35": "nm", "36": "ny", "37": "nc", "38": "nd",
	"39": "oh", "40": "ok", "41": "or", "42": "pa", "44": "ri",
	"45": "sc", "46": "sd", "47": "tn", "48": "tx", "49": "ut",
	"50": "vt", "51": "va", "53": "wa", "54": "wv", "55": "wi",
	"56": "wy", "72": "pr",
}

// statePostalToSlug maps the 2-letter postal abbreviations that
// appear in CBSA titles (e.g., "NY-NJ-PA" in "New York-Newark-Jersey
// City, NY-NJ") to the corresponding state region slug. Used to set
// parent edges on auto-generated MSA regions.
var statePostalToSlug = map[string]string{
	"AL": "al", "AK": "ak", "AZ": "az", "AR": "ar", "CA": "ca-state",
	"CO": "co", "CT": "ct", "DE": "de-state", "DC": "dc", "FL": "fl",
	"GA": "ga", "HI": "hi", "ID": "id", "IL": "il", "IN": "in",
	"IA": "ia", "KS": "ks", "KY": "ky", "LA": "la-state", "ME": "me",
	"MD": "md", "MA": "ma", "MI": "mi", "MN": "mn", "MS": "ms",
	"MO": "mo", "MT": "mt", "NE": "ne", "NV": "nv", "NH": "nh",
	"NJ": "nj", "NM": "nm", "NY": "ny", "NC": "nc", "ND": "nd",
	"OH": "oh", "OK": "ok", "OR": "or", "PA": "pa", "RI": "ri",
	"SC": "sc", "SD": "sd", "TN": "tn", "TX": "tx", "UT": "ut",
	"VT": "vt", "VA": "va", "WA": "wa", "WV": "wv", "WI": "wi",
	"WY": "wy", "PR": "pr",
}

// ctLegacyCounties is the set of Connecticut's 8 retired legacy county
// FIPS (09001–09015). On 2022-06-06 the Census Bureau replaced these
// with CT's 9 planning regions as county-equivalents (FIPS 09110–09190).
//
// The two ETL sources we join on county GEOID are now different
// vintages: the July-2023 CBSA delineation (which builds countyToMSA)
// keys CT metros by the *new* planning-region GEOIDs, while the 2020
// ZCTA→county relationship file still keys CT ZCTAs by these *legacy*
// codes. So countyToMSA[<legacy CT county>] always misses and every CT
// ZCTA ZIP falls through the county→MSA tier to the bare `ct` state
// anchor — e.g. Stamford 06902 surfaced no Bridgeport-Stamford / NY
// Tri-State orgs.
//
// The 2020 ZCTA→county file is a frozen decennial product with no
// drop-in planning-region successor (next ZCTA refresh is 2030), so the
// fix can't be a source bump. Instead reconcileCTLegacyCounties
// (crosswalk.go) re-resolves these ZIPs through the current-vintage HUD
// crosswalk, which already uses the planning-region FIPS.
//
// CT is the only county recode in the 2020→2023 source-vintage gap, so
// this hardcoded set is the whole problem today; future recodes will
// join the class as the frozen ZCTA file ages toward 2030. The rationale
// for keeping the trigger scoped (and why NOT to broaden it to a blanket
// "ZCTA-at-state → prefer HUD") lives in etl/SOURCES.md §"Known
// data-vintage gaps" — the single source of truth for this decision.
var ctLegacyCounties = map[string]struct{}{
	"09001": {}, // Fairfield
	"09003": {}, // Hartford
	"09005": {}, // Litchfield
	"09007": {}, // Middlesex
	"09009": {}, // New Haven
	"09011": {}, // New London
	"09013": {}, // Tolland
	"09015": {}, // Windham
}
