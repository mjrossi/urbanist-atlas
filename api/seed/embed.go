// Package seedfs holds the embedded bundle of region/postal/org
// files. It exists for the //go:embed directive — Go's embed
// directive can only reach files at or below the source file's
// directory, so this tiny package sits alongside the .toml/.csv
// data it embeds.
//
// Consumers (currently cmd/server) import FS and pass it to
// internal/seedfiles.BuildMemStore. Tests that want to override the
// bundle pass os.DirFS(path) instead.
package seedfs

import "embed"

// FS embeds every region taxonomy TOML, every postal-code CSV, and
// the curated orgs file. The PT validation fixtures
// (regions_pt.toml, postal_codes_pt.csv, orgs_pt.toml) are embedded
// alongside the active bundle but never opened by the runtime —
// internal/seedfiles only iterates the countries it lists, currently
// US and CA. Bundling the fixtures lets a future PT reactivation
// flip a single switch in seedfiles.countries with no embed change.
//
//go:embed regions_*.toml postal_codes_*.csv orgs*.toml
var FS embed.FS
