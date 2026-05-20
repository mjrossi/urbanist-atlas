// Package loaddata orchestrates the full bundled-seed import in the
// order the schema requires: regions taxonomy first (so leaf slugs
// resolve), then postal codes (which reference those slugs), then the
// org seed (which attaches to any node in the region graph). LoadAll
// is the single entry point; cmd/server/loaddata.go wraps it as the
// `loaddata` subcommand and the integration suite uses it directly.
//
// Adding a new country: drop seed/regions_<cc>.toml and
// seed/postal_codes_<cc>.csv into api/seed/, then append a
// {code, regionFiles, postal} entry to countries below. The pipeline
// test exercises every listed country end-to-end, so coverage stays
// automatic. If the new country has a state-tier file (e.g.,
// regions_<cc>_states.toml), list it before the main file in
// regionFiles so the main file's leaves can parent under the states.
package loaddata

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mjrossi/urbanist-atlas/api/internal/loadpostal"
	"github.com/mjrossi/urbanist-atlas/api/internal/loadregions"
	"github.com/mjrossi/urbanist-atlas/api/internal/seed"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// countries lists every country whose bundled seed (regions + postal
// codes) gets loaded by LoadAll.
//
//   - code:        canonical upper-case country code stamped on every row.
//   - regionFiles: file suffixes for regions_<suffix>.toml, in load
//     order. Earlier files load first; later files may reference
//     earlier-loaded regions as parents via cross-file resolution
//     (see internal/loadregions/write.go's RegionIDBySlug fallback).
//     For US/CA the convention is to load the state/province tier
//     before the main file so leaves can parent under them.
//   - postal:      file suffix for postal_codes_<suffix>.csv.
var countries = []struct {
	code        string
	regionFiles []string
	postal      string
}{
	{"US", []string{"us_states", "us_multistate", "us_msas", "us"}, "us"},
	{"CA", []string{"ca_provinces", "ca"}, "ca"},
	{"PT", []string{"pt"}, "pt"},
}

// Countries returns the country codes whose seed files LoadAll loads,
// in dependency order. Exposed so test code (and any future tooling
// that needs to enumerate the bundle) can stay in sync with the source
// of truth above without re-stating the list.
func Countries() []string {
	out := make([]string, 0, len(countries))
	for _, c := range countries {
		out = append(out, c.code)
	}
	return out
}

// LoadAll loads every bundled seed file in seedDir into the database
// pointed to by pool. The chain is:
//
//   - for each country in `countries`: loadregions.LoadFile
//   - for each country in `countries`: loadpostal.LoadFile
//   - seed.LoadFile (orgs.toml)
//
// All three underlying loaders are upsert-based, so LoadAll is safe to
// re-run; the pipeline integration test asserts idempotence.
//
// On any step's failure LoadAll returns immediately with a wrapped
// error naming the failing step (e.g. "loaddata: regions PT: ...").
// Successful steps stay committed — partial state is intentional so
// operators can fix the bad file and re-run rather than starting from
// scratch.
func LoadAll(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, seedDir string) error {
	for _, c := range countries {
		for _, suffix := range c.regionFiles {
			path := filepath.Join(seedDir, "regions_"+suffix+".toml")
			if _, err := loadregions.LoadFile(ctx, pool, logger, path, c.code); err != nil {
				return fmt.Errorf("loaddata: regions %s/%s: %w", c.code, suffix, err)
			}
		}
	}
	for _, c := range countries {
		path := filepath.Join(seedDir, "postal_codes_"+c.postal+".csv")
		if _, err := loadpostal.LoadFile(ctx, pool, logger, path, atlas.Country(c.code)); err != nil {
			return fmt.Errorf("loaddata: postal %s: %w", c.code, err)
		}
	}
	orgs := filepath.Join(seedDir, "orgs.toml")
	if _, err := seed.LoadFile(ctx, pool, logger, orgs); err != nil {
		return fmt.Errorf("loaddata: orgs: %w", err)
	}
	return nil
}
