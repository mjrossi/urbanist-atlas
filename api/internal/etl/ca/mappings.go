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
// api/seed/regions_ca_cma_overrides.toml, read by etl.ReadOverrides
// and applied by assignCMAs — so a CA metro slug correction
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

// The FSA → CMA assignment used to live here as a coarse
// `fsaPrefixToCMA` table mapping an FSA's first one or two characters to
// one of the seven biggest metros. It was replaced by a max-overlap
// spatial join of the boundary-file polygons (spatial.go), which resolves
// all ~41 CMAs and can separate adjacent metros that share an FSA prefix
// (e.g. Victoria and Nanaimo, both V9). See issue #81 and
// docs/superpowers/specs/2026-05-19-postal-coverage-design.md (Open
// Question §4, now resolved).
