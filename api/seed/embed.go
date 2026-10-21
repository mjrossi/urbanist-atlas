// Package seedfs holds the embedded bundle of region, postal-code and org
// seed files. It sits beside the data because //go:embed can only reach
// files at or below the source file's directory.
package seedfs

import "embed"

// FS embeds every region TOML, postal-code CSV and orgs file. The PT
// validation fixtures are embedded too but never opened: seedfiles only
// loads the countries it lists.
//
//go:embed regions_*.toml postal_codes_*.csv orgs*.toml
var FS embed.FS
