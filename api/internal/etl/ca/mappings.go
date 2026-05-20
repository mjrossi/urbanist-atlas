package ca

// fsaToLeaf maps specific 3-character FSA codes to curated city-leaf
// slugs in api/seed/regions_ca.toml. When adding a new curated CA
// city leaf, add the FSAs that fall in it here.
//
// The list is intentionally short: it mirrors the existing fixture
// FSAs in postal_codes_ca.csv. Future slices that add more curated CA
// cities should expand this map alongside the leaf row.
var fsaToLeaf = map[string]string{
	// Vancouver, BC (V5C, V5L, V5R fall under Burnaby; V6X/V6Y under
	// Richmond — see existing fixture)
	"V6B": "vancouver",
	"V6E": "vancouver",
	"V6A": "vancouver",
	// Burnaby
	"V5C": "burnaby",
	// Richmond
	"V6X": "richmond",
	// Toronto, ON
	"M5V": "toronto",
	"M4W": "toronto",
	"M6J": "toronto",
	// Montréal, QC
	"H2X": "montreal",
	"H3A": "montreal",
}

// cmaOverrides maps the 3-digit StatsCan CMA UID to a curated
// slug + name + kind for well-known Canadian metros we want to pin
// over the auto-generated defaults. CBSA-style fields:
//
//   - Slug — preserves existing seed slugs (toronto-cma,
//     montreal-cma, metro-vancouver) and shortens long ones
//     (ottawa-gatineau-cma vs. the raw "Ottawa - Gatineau" slugify).
//   - Name — display name on metros pages. Defaults to the cleaned
//     CMA name from the DBF; overrides give nicer "Greater Toronto
//     Area"-style labels.
//   - Kind — defaults to "ca:cma". Override to "ca:regional-district"
//     for Metro Vancouver (matches StatsCan's regional-district
//     vocabulary the existing curated record used).
//
// Empty fields fall back to the auto-generated values.
type cmaOverride struct {
	Slug string
	Name string
	Kind string
}

var cmaOverrides = map[string]cmaOverride{
	"535": {Slug: "toronto-cma", Name: "Greater Toronto Area"},
	"462": {Slug: "montreal-cma", Name: "Greater Montréal"},
	"933": {Slug: "metro-vancouver", Name: "Metro Vancouver", Kind: "ca:regional-district"},
	"505": {Slug: "ottawa-gatineau-cma", Name: "Ottawa-Gatineau"},
}

// provinceUIDToSlug maps 2-digit Statistics Canada province/territory
// codes to the province region slugs in api/seed/regions_ca_provinces.toml.
//
// "nl-province" is suffix-disambiguated to avoid the bare "nl" slug
// colliding with the country code for the Netherlands. Other province
// slugs use their lowercase 2-letter postal abbreviation.
var provinceUIDToSlug = map[string]string{
	"10": "nl-province", // Newfoundland and Labrador
	"11": "pe",          // Prince Edward Island
	"12": "ns",          // Nova Scotia
	"13": "nb",          // New Brunswick
	"24": "qc",          // Québec
	"35": "on",          // Ontario
	"46": "mb",          // Manitoba
	"47": "sk",          // Saskatchewan
	"48": "ab",          // Alberta
	"59": "bc",          // British Columbia
	"60": "yt",          // Yukon
	"61": "nt",          // Northwest Territories
	"62": "nu",          // Nunavut
}

// fsaPrefixToCMA hand-codes a coarse "FSA first-1-or-2-char →
// curated CMA slug" mapping for the major Canadian metros where a
// per-FSA spatial join (the kind the PCCF would give us) isn't
// available without restricted-licence data.
//
// Coverage is intentionally limited to the highest-population CMAs
// where prefix-based assignment is reasonably accurate:
//
//   - M*       — Toronto CMA (Toronto proper)
//   - H*       — Montréal CMA (Montréal Island)
//   - V5*..V7* — Metro Vancouver (Vancouver, Burnaby, Richmond, etc.)
//   - K1*..K2* — Ottawa-Gatineau CMA (Ottawa side)
//   - J8*..J9* — Ottawa-Gatineau CMA (Gatineau side)
//   - T2*..T3* — Calgary CMA
//   - T5*..T6* — Edmonton CMA
//
// Lookup logic in crosswalk.go: try the 2-character prefix first
// (e.g., "M5"), then the 1-character (e.g., "M"). FSAs without a
// match fall through to province.
//
// Future slices with PCCF or spatial-join data can replace this with
// a per-FSA mapping for full CMA accuracy.
var fsaPrefixToCMA = map[string]string{
	// Toronto CMA — M (Toronto proper) + suburbs in the L block
	// (Pickering/Ajax/Whitby/Oshawa L1; Markham/Stouffville L3;
	// Mississauga/Brampton/Vaughan L4-L6; Halton Hills/Burlington
	// straddle L7 → leave to province for safety).
	"M":  "toronto-cma",
	"L1": "toronto-cma",
	"L3": "toronto-cma",
	"L4": "toronto-cma",
	"L5": "toronto-cma",
	"L6": "toronto-cma",
	// Hamilton CMA — L8, L9 cover Hamilton proper.
	"L8": "hamilton-cma",
	"L9": "hamilton-cma",
	// Montréal CMA — H is Montréal Island; J-suburbs straddle metros
	// and are left to province for safety.
	"H": "montreal-cma",
	// Metro Vancouver — V5-V7 in the BC metro.
	"V5": "metro-vancouver",
	"V6": "metro-vancouver",
	"V7": "metro-vancouver",
	// Ottawa-Gatineau CMA — K1-K2 (Ontario side), J8-J9 (Quebec).
	"K1": "ottawa-gatineau-cma",
	"K2": "ottawa-gatineau-cma",
	"J8": "ottawa-gatineau-cma",
	"J9": "ottawa-gatineau-cma",
	// Calgary CMA — T2-T3 cover Calgary proper.
	"T2": "calgary-cma",
	"T3": "calgary-cma",
	// Edmonton CMA — T5-T6 cover Edmonton proper.
	"T5": "edmonton-cma",
	"T6": "edmonton-cma",
}
