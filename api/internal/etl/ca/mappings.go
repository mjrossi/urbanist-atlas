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

// CMA editorial overrides (3-digit StatsCan CMA UID → curated
// slug/name/kind) used to live here as a compiled
// `cmaOverrides` map. They were lifted into data —
// api/seed/regions_ca_cma_overrides.toml, read by ReadCMAOverrides in
// output.go and applied by assignCMAs — so a CA metro slug correction
// is a data edit (no Go change + recompile), symmetric with the US
// side's regions_us_msa_overrides.toml. See ETL-04b / plan 03-04.

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
// a per-FSA mapping for full CMA accuracy. The trade-off is discussed
// in detail in docs/superpowers/specs/2026-05-19-postal-coverage-design.md
// under "FSA → CMA mapping without PCCF" (Open Question §4).
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
