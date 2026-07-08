// Package etl wraps the operator-side data pipeline that reshapes
// upstream postal-code and metro reference data (Census ZCTA + CBSA,
// Statistics Canada PCCF + CMA, etc.) into the seed file shapes that
// internal/seedfiles.BuildMemStore loads into the in-memory FileStore
// at API boot.
//
// The pipeline is two-phase:
//
//  1. download — fetch upstream files into etl/sources/<country>/,
//     validate checksums against etl/SOURCES.md.
//  2. regenerate — parse the staged sources and emit deterministic
//     TOML/CSV under api/seed/ (e.g., regions_us_msas.toml,
//     postal_codes_us.csv).
//
// Output must be byte-identical given the same upstream inputs: rows
// sorted by primary key (postal_code or slug), no embedded timestamps,
// LF line endings, trailing newline. This keeps git diffs signal-rich
// on intentional vintage upgrades and lets CI gate on determinism.
//
// Layout:
//
//   - SourceDescriptor — declares an expected upstream file (path,
//     URL, sha256, vintage label).
//   - OutputTarget     — declares a generated seed artifact (path
//     relative to api/seed/, expected row count band, format hint).
//   - Country          — bundles a country's descriptors and targets
//     so the download and regenerate flows can iterate uniformly.
//
// This package ships in slice #7.5.1 as a skeleton with no concrete
// country plans yet. Slice #7.5.3 fills in the US plan (Census CBSA +
// ZCTA crosswalks → MSAs + ZIPs). Slice #7.5.4 fills in the CA plan
// (StatsCan CMA + PCCF → CMAs + FSAs). The cli wiring lives at
// api/cmd/server/etl.go.
package etl

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
)

// Target selects which seed outputs a regenerate (or download) operates
// on. The default is TargetAll. TargetRegions exists so CI can refresh
// the HUD-free region files without touching the HUD-dependent postal
// CSV (US) or needing the 155 MB CA FSA source.
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
// find in etl/sources/<country>/. Concrete instances live in per-country
// plans (added in slices #7.5.3 and #7.5.4).
type SourceDescriptor struct {
	// Filename is the basename the download step writes (and the
	// regenerate step reads) under etl/sources/<country>/.
	Filename string
	// URL is the canonical upstream location. Logged in error
	// messages so a manual re-fetch is one curl away.
	URL string
	// SHA256 is the hex-encoded checksum the downloaded file must
	// match. The expected value is mirrored in etl/SOURCES.md so
	// reviewers can confirm vintage without running the pipeline.
	SHA256 string
	// Vintage is a human-readable label for the data release
	// (e.g., "Census 2020", "StatsCan 2025-Q1"). Logged when the
	// download step writes the file.
	Vintage string
	// Optional marks a source the download step must NOT fail on — e.g.
	// HUD, whose canonical URL is an account-gated landing page, not a
	// direct file. Download skips Optional sources with a notice; the
	// operator fetches them by hand. Regenerate already tolerates their
	// absence.
	Optional bool
	// Targets lists which output Targets this source feeds. Empty means
	// it feeds every target. Download uses this to fetch only what a
	// given --target needs (e.g. --target=regions skips the CA FSA).
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
// api/seed/. The expected row count band is purely informational —
// it surfaces in the regenerate step's summary log so an order-of-
// magnitude mismatch (e.g., 33k US ZCTAs collapses to 100 rows)
// is obvious even without diff inspection.
type OutputTarget struct {
	// Path is relative to api/seed/ (e.g., "regions_us_msas.toml",
	// "postal_codes_us.csv").
	Path string
	// Format is "toml" or "csv". Determines the writer used by the
	// regenerate step and the deterministic-ordering convention
	// (slug for TOML, postal_code for CSV).
	Format string
	// MinRows / MaxRows bracket the expected output cardinality.
	// Zero means unbounded.
	MinRows int
	MaxRows int
}

// Country bundles a country's expected upstream sources, the targets
// it generates, and the Regenerate hook that performs the
// transformation. Concrete plans (US, CA) register via init() blocks
// in internal/etl/<cc>/ subpackages; the cmd/server etl subcommand
// dispatches into Plans[code].
type Country struct {
	// Code is the canonical upper-case country code (e.g., "US",
	// "CA").
	Code string
	// SourcesDir is the per-country subdirectory under etl/sources/
	// where the download step writes (defaults to lowercase Code).
	SourcesDir string
	// Sources lists every upstream file the plan needs.
	Sources []SourceDescriptor
	// Targets lists every seed artifact the regenerate step writes.
	Targets []OutputTarget
	// Regenerate parses the staged source files in srcDir and writes
	// the deterministic seed outputs (TOML + CSV) into outDir. May be
	// nil for plans whose regenerate flow isn't implemented yet — in
	// that case the cli stub returns an error indicating which slice
	// is expected to land it.
	Regenerate func(ctx context.Context, srcDir, outDir string, target Target, logger *slog.Logger) error
}

// Plans is the registered set of country plans the ETL subcommand
// dispatches against. Empty at the foundation slice (#7.5.1); slices
// #7.5.3 and #7.5.4 populate the US and CA entries.
var Plans = map[string]Country{}
