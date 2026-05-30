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
// regions_us.toml (currently `cook-county` and `lake-county-in`).
// Used as a fallback when no curated city-place leaf matches a ZCTA.
var countyToLeaf = map[string]string{
	"17031": "cook-county",    // Cook County, IL (contains Chicago)
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

// zipAnchorOverride pins specific ZIPs to a curated anchor slug,
// overriding whatever the ZCTA/HUD county crosswalk would otherwise
// produce. It is applied last, in WritePostalCodesCSV, so it wins over
// both the primary ZCTA pass and the HUD backfill. Editorial use only:
// reserve it for ZIPs the upstream crosswalk mis-anchors, not as a
// general substitute for the place/county tiers.
//
// Connecticut driver (slice: Stamford/Tri-State): Census adopted CT's
// nine planning regions as county-equivalents starting with the 2022
// vintage. The 2020 ZCTA-to-county relationship file still keys CT
// ZCTAs by the *old* county FIPS (Fairfield County = 09001), but the
// July-2023 CBSA delineation that builds countyToMSA references the new
// planning-region GEOIDs instead. The old FIPS therefore isn't in
// countyToMSA, so the residential (ZCTA-sourced) Fairfield ZIPs fell
// through the county→MSA tier to the bare `ct` state anchor — while the
// HUD-sourced P.O.-box ZIPs in the very same towns, keyed by the newer
// FIPS, correctly resolved to bridgeport-ct-metro. That split is why
// Stamford 06902 surfaced no Tri-State orgs.
//
// Re-anchoring these lower-Fairfield ("Gold Coast") residential ZIPs at
// bridgeport-ct-metro restores the smallest-anchor invariant: the metro
// is Census CBSA 14860 (Bridgeport-Stamford-Danbury) and parents under
// nyc-tristate + ct (see regions_us_msa_overrides.toml), so these ZIPs
// now surface both the Tri-State region and Connecticut state orgs.
//
// Scope is the towns squarely in the NY commuter shed — Greenwich,
// Stamford, Darien, New Canaan, Norwalk, Westport, Weston, Wilton.
// Mid/upper Fairfield (Danbury, Ridgefield, …) is intentionally left at
// its current anchor; widen this map if that coverage is wanted.
var zipAnchorOverride = map[string]string{
	// Greenwich
	"06807": "bridgeport-ct-metro", // Cos Cob
	"06830": "bridgeport-ct-metro", // Greenwich
	"06831": "bridgeport-ct-metro", // Greenwich (backcountry)
	"06870": "bridgeport-ct-metro", // Old Greenwich
	"06878": "bridgeport-ct-metro", // Riverside
	// Stamford
	"06901": "bridgeport-ct-metro",
	"06902": "bridgeport-ct-metro",
	"06903": "bridgeport-ct-metro",
	"06905": "bridgeport-ct-metro",
	"06906": "bridgeport-ct-metro",
	"06907": "bridgeport-ct-metro",
	// Darien
	"06820": "bridgeport-ct-metro",
	// New Canaan
	"06840": "bridgeport-ct-metro",
	// Norwalk
	"06850": "bridgeport-ct-metro",
	"06851": "bridgeport-ct-metro",
	"06853": "bridgeport-ct-metro",
	"06854": "bridgeport-ct-metro",
	"06855": "bridgeport-ct-metro",
	// Westport / Weston / Wilton
	"06880": "bridgeport-ct-metro", // Westport
	"06883": "bridgeport-ct-metro", // Weston
	"06897": "bridgeport-ct-metro", // Wilton
}
