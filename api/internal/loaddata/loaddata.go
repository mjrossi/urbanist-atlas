// Package loaddata orchestrates the full bundled-seed import in the
// order the schema requires: regions taxonomy first (so leaf slugs
// resolve), then postal codes (which reference those slugs), then the
// org seed (which attaches to any node in the region graph). LoadAll
// is the single entry point; cmd/server/loaddata.go wraps it as the
// `loaddata` subcommand and the integration suite uses it directly.
//
// Adding a new country: drop seed/regions_<cc>.toml and
// seed/postal_codes_<cc>.csv into api/seed/, then append a {code,
// suffix} pair to countries below. The pipeline test exercises every
// listed country end-to-end, so coverage stays automatic.
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
// codes) gets loaded by LoadAll. `code` is the canonical upper-case
// country code stamped on every row; `suffix` matches the seed
// filename convention (regions_<suffix>.toml, postal_codes_<suffix>.csv).
var countries = []struct {
	code   string
	suffix string
}{
	{"US", "us"},
	{"CA", "ca"},
	{"PT", "pt"},
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
		path := filepath.Join(seedDir, "regions_"+c.suffix+".toml")
		if _, err := loadregions.LoadFile(ctx, pool, logger, path, c.code); err != nil {
			return fmt.Errorf("loaddata: regions %s: %w", c.code, err)
		}
	}
	for _, c := range countries {
		path := filepath.Join(seedDir, "postal_codes_"+c.suffix+".csv")
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
