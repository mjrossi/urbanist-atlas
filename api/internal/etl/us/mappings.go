package us

// placeToLeaf maps 7-digit Census place GEOIDs to the curated city-leaf
// slugs in api/seed/regions_us.toml. A new curated city leaf needs its
// GEOID_PLACE_20 from the ZCTA-to-place crosswalk added here.
//
// The crosswalk picks each ZCTA's primary place (largest AREALAND_PART),
// so a ZIP only anchors at a leaf when that city is its primary place:
// 07302 overlaps Hoboken but its primary place is Jersey City. New York
// city (3651000) is deliberately absent: NYC ZIPs anchor at borough
// leaves through nycBoroughCounty, since boroughs are counties.
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

// nycBoroughCounty maps the five NYC borough county GEOIDs to
// borough-leaf slugs. NYC is the only US city split sub-municipally in
// v1 (docs/region-graph.md §8).
var nycBoroughCounty = map[string]string{
	"36005": "bronx",         // Bronx County, NY
	"36047": "brooklyn",      // Kings County, NY
	"36061": "manhattan",     // New York County, NY
	"36081": "queens",        // Queens County, NY
	"36085": "staten-island", // Richmond County, NY
}

// countyToLeaf maps non-NYC county GEOIDs to curated county leaves,
// consulted before the MSA tier. The six Illinois entries are the RTA
// service area: collar-county suburbs reach Illinois statewide orgs
// through rta-service-area → il, because the IL+IN Chicago metro can't
// carry a state edge (docs/region-graph.md §1).
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
// slugs in api/seed/regions_us_states.toml: 50 states, DC and Puerto
// Rico. "ca-state", "de-state" and "la-state" carry a suffix so the bare
// slugs stay clear of Canada, Germany and Los Angeles.
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

// statePostalToSlug maps the postal abbreviations in CBSA titles (the
// "NY-NJ" in "New York-Newark-Jersey City, NY-NJ") to state slugs, for
// the parent edges of generated MSA regions.
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

// ctLegacyCounties is Connecticut's 8 retired county FIPS (09001–09015).
// Census replaced them with 9 planning regions (09110–09190) in 2022, so
// the 2023 CBSA delineation keys CT metros by planning region while the
// frozen 2020 ZCTA→county file still uses these codes, and every CT ZCTA
// would fall through to the bare `ct` anchor. reconcileCTLegacyCounties
// re-resolves them through HUD; see etl/SOURCES.md "Known data-vintage
// gaps" for why the trigger stays this narrow.
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
