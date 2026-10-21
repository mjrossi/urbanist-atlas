// Package etl wraps the operator-side data pipeline that reshapes
// upstream postal-code and metro reference data (Census ZCTA + CBSA,
// HUD USPS crosswalks, Statistics Canada FSA + CMA boundaries) into the
// seed files that internal/seedfiles.BuildMemStore loads at API boot.
//
// The pipeline is two-phase: download fetches upstream files into
// etl/sources/<country>/ and checks them against the SHA256 pinned in
// each plan (mirrored in etl/SOURCES.md); regenerate parses the staged
// sources and writes TOML/CSV under api/seed/.
//
// Output must be byte-identical given the same upstream inputs: rows
// sorted by primary key, no embedded timestamps, LF line endings,
// trailing newline. That keeps vintage-bump diffs signal-rich and lets
// CI gate on determinism. Country plans live in internal/etl/<cc>/; see
// docs/etl-architecture.md.
package etl

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
)

// Target selects which seed outputs a regenerate (or download) operates
// on. TargetRegions exists so CI can refresh the HUD-free region files
// without the HUD-dependent US postal CSV or the 155 MB CA FSA source.
type Target string

const (
	TargetAll     Target = "all"
	TargetRegions Target = "regions"
	TargetPostal  Target = "postal"
)

// Regions reports whether this target includes the region (TOML) outputs.
func (t Target) Regions() bool { return t == TargetAll || t == TargetRegions }

// Postal reports whether this target includes the postal-code (CSV) outputs.
func (t Target) Postal() bool { return t == TargetAll || t == TargetPostal }

// ParseTarget validates a CLI --target value.
func ParseTarget(s string) (Target, error) {
	switch Target(s) {
	case TargetAll, TargetRegions, TargetPostal:
		return Target(s), nil
	default:
		return "", fmt.Errorf("invalid target %q (want one of: all, regions, postal)", s)
	}
}

// SourceDescriptor names an upstream file the ETL pipeline expects to
// find in etl/sources/<country>/.
type SourceDescriptor struct {
	// Filename is the basename under etl/sources/<country>/.
	Filename string
	// URL is the canonical upstream location, logged in errors so a
	// manual re-fetch is one curl away.
	URL string
	// SHA256 is the hex checksum the downloaded file must match, mirrored
	// in etl/SOURCES.md so reviewers can confirm the vintage.
	SHA256 string
	// Vintage labels the data release (e.g. "Census 2020").
	Vintage string
	// Optional marks a source download skips instead of failing on, e.g.
	// HUD, whose canonical URL is an account-gated landing page rather
	// than a file. The operator fetches it by hand.
	Optional bool
	// Targets lists which output Targets this source feeds; empty means
	// every target.
	Targets []Target
}

// FeedsTarget reports whether this source is needed to produce target.
func (s SourceDescriptor) FeedsTarget(target Target) bool {
	if len(s.Targets) == 0 || target == TargetAll {
		return true
	}
	return slices.Contains(s.Targets, target)
}

// OutputTarget names a file the regenerate step writes under
// api/seed/.
type OutputTarget struct {
	// Path is relative to api/seed/ (e.g., "regions_us_msas.toml",
	// "postal_codes_us.csv").
	Path string
}

// Country bundles a country's upstream sources, the targets it
// generates, and the Regenerate hook. Plans self-register in Plans from
// init() in internal/etl/<cc>/.
type Country struct {
	// Code is the upper-case country code (e.g. "US").
	Code string
	// SourcesDir is the subdirectory of etl/sources/ for this country.
	SourcesDir string
	// Sources lists every upstream file the plan needs.
	Sources []SourceDescriptor
	// Targets lists every seed artifact the regenerate step writes.
	Targets []OutputTarget
	// Regenerate parses the staged sources in srcDir and writes the seed
	// outputs into outDir. Nil means the plan has no regenerate step yet.
	Regenerate func(ctx context.Context, srcDir, outDir string, target Target, logger *slog.Logger) error
}

// Plans is the registered set of country plans, keyed by Country.Code.
var Plans = map[string]Country{}
