package ca

// fsaToLeaf maps 3-character FSA codes to curated city-leaf slugs in
// api/seed/regions_ca.toml. A new curated CA city leaf needs its FSAs
// added here.
var fsaToLeaf = map[string]string{
	// Vancouver, BC
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

// provinceUIDToSlug maps 2-digit Statistics Canada province/territory
// codes to the province region slugs in api/seed/regions_ca_provinces.toml.
//
// "nl-province" carries a suffix so it can't collide with "nl", the
// Netherlands' country code. The others are the postal abbreviation.
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
