// Package seedfiles parses the bundled seed data
// (regions_*.toml, postal_codes_*.csv, orgs.toml) and builds an
// in-memory atlas.MemStore. It operates over an fs.FS so the same
// loader works against the production embed (api/seed/embed.go) and
// against a directory on disk (os.DirFS(seedDir)).
//
// Adding a new country: drop seed/regions_<cc>.toml and
// seed/postal_codes_<cc>.csv into api/seed/, then append a
// {code, regionFiles, postal} entry to countries below. If the new
// country has a state-tier file (e.g., regions_<cc>_states.toml),
// list it before the main file in regionFiles so the main file's
// leaves can parent under the states.
package seedfiles

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// countries lists every country whose bundled seed gets loaded by
// BuildMemStore.
//
//   - code:        canonical upper-case country code stamped on every region.
//   - regionFiles: file suffixes for regions_<suffix>.toml, in load
//     order. Earlier files load first; later files may reference
//     earlier-loaded regions as parents.
//   - postal:      file suffix for postal_codes_<suffix>.csv.
var countries = []struct {
	code        string
	regionFiles []string
	postal      string
}{
	{"US", []string{"us_states", "us_multistate", "us_msas", "us"}, "us"},
	{"CA", []string{"ca_provinces", "ca_cmas", "ca"}, "ca"},
	// PT was loaded through slice #4.6 as a region-graph validation
	// fixture and dropped from the user-facing pipeline by slice #25.
	// The seed files (regions_pt.toml, postal_codes_pt.csv) stay in
	// api/seed/ for tests that load them explicitly via ParseRegions/
	// ParsePostal; a future v1.1+ slice can reintroduce PT here when
	// the editorial coverage is ready to ship.
}

// Countries returns the country codes BuildMemStore loads, in
// dependency order.
func Countries() []string {
	out := make([]string, 0, len(countries))
	for _, c := range countries {
		out = append(out, c.code)
	}
	return out
}

// BuildMemStore parses every bundled seed file from seedFS and
// returns a populated atlas.MemStore. Synthetic int64 IDs are
// assigned to regions and orgs in load order; the wire contract
// identifies both by slug, so the IDs are stable-within-process and
// process-only.
func BuildMemStore(logger *slog.Logger, seedFS fs.FS) (*atlas.MemStore, error) {
	store := atlas.NewMemStore()
	regionIDBySlug := map[string]int64{}
	var nextRegionID int64 = 1

	// Stage 1: regions, in dependency order.
	for _, c := range countries {
		for _, suffix := range c.regionFiles {
			path := "regions_" + suffix + ".toml"
			regions, err := readRegions(seedFS, path)
			if err != nil {
				return nil, fmt.Errorf("seedfiles: regions %s/%s: %w", c.code, suffix, err)
			}
			if err := DetectCycles(regions); err != nil {
				return nil, fmt.Errorf("seedfiles: regions %s/%s: %w", c.code, suffix, err)
			}
			for _, r := range regions {
				if _, dup := regionIDBySlug[r.Slug]; dup {
					return nil, fmt.Errorf("seedfiles: duplicate region slug %q across files", r.Slug)
				}
				for _, ps := range r.ParentSlugs {
					if _, ok := regionIDBySlug[ps]; !ok {
						return nil, fmt.Errorf("seedfiles: region %q references unknown parent slug %q (load the defining file first)", r.Slug, ps)
					}
				}
				r.ID = nextRegionID
				r.Country = atlas.Country(c.code)
				regionIDBySlug[r.Slug] = nextRegionID
				nextRegionID++
				store.AddRegion(r)
			}
		}
	}

	// Stage 2: postal codes, country-by-country.
	for _, c := range countries {
		path := "postal_codes_" + c.postal + ".csv"
		rows, err := readPostal(seedFS, path, atlas.Country(c.code))
		if err != nil {
			return nil, fmt.Errorf("seedfiles: postal %s: %w", c.code, err)
		}
		for _, row := range rows {
			leafID, ok := regionIDBySlug[row.LeafRegionSlug]
			if !ok {
				return nil, fmt.Errorf("seedfiles: postal %s/%s: leaf_region_slug %q not found", row.Country, row.PostalCode, row.LeafRegionSlug)
			}
			store.AddPostalCode(row.Country, row.PostalCode, leafID)
		}
		if logger != nil {
			logger.Debug("seedfiles: postal loaded", "country", c.code, "rows", len(rows))
		}
	}

	// Stage 3: orgs.
	orgs, err := readOrgs(seedFS, "orgs.toml")
	if err != nil {
		return nil, fmt.Errorf("seedfiles: orgs: %w", err)
	}
	var nextOrgID int64 = 1
	for _, entry := range orgs {
		ids, err := resolveOrgRegions(entry, regionIDBySlug)
		if err != nil {
			return nil, fmt.Errorf("seedfiles: orgs: %w", err)
		}
		// added_at is required; a missing TOML key parses as the zero
		// toml.LocalDate (Year == 0). Reject loudly with the offending
		// slug so an operator can fix the seed file without grep.
		if entry.AddedAt.Year == 0 {
			return nil, fmt.Errorf("seedfiles: org %q: missing required added_at", entry.Slug)
		}
		o := entry.Org
		o.ID = nextOrgID
		// added_at parses as a date-only toml.LocalDate on the entry
		// wrapper (atlas.Org.AddedAt is toml:"-"); pin it to midnight
		// UTC.
		o.AddedAt = time.Date(entry.AddedAt.Year, time.Month(entry.AddedAt.Month), entry.AddedAt.Day, 0, 0, 0, 0, time.UTC)
		store.AddOrg(o, ids)
		nextOrgID++
	}
	if logger != nil {
		logger.Info("seedfiles: filestore built",
			"regions", len(regionIDBySlug),
			"orgs", len(orgs),
		)
	}
	return store, nil
}

// resolveOrgRegions returns the int64 region IDs for an org's
// region_slugs, sorted ascending so the wire shape matches the
// pre-rewrite contract. Unknown slugs are a hard error.
func resolveOrgRegions(o OrgEntry, regionIDBySlug map[string]int64) ([]int64, error) {
	out := make([]int64, 0, len(o.RegionSlugs))
	seenIDs := map[int64]struct{}{}
	var missing []string
	for _, slug := range o.RegionSlugs {
		id, ok := regionIDBySlug[slug]
		if !ok {
			missing = append(missing, slug)
			continue
		}
		if _, dup := seenIDs[id]; dup {
			continue
		}
		seenIDs[id] = struct{}{}
		out = append(out, id)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("org %q references unknown region slug(s) %v", o.Slug, missing)
	}
	slices.Sort(out)
	return out, nil
}

// openFile is a small wrapper that turns fs.ErrNotExist into a
// pointable "seed file missing: <path>" error for the operator.
func openFile(seedFS fs.FS, path string) (fs.File, error) {
	f, err := seedFS.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("seed file missing: %s", path)
		}
		return nil, err
	}
	return f, nil
}

func readRegions(seedFS fs.FS, path string) ([]atlas.Region, error) {
	f, err := openFile(seedFS, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseRegions(f)
}

func readPostal(seedFS fs.FS, path string, country atlas.Country) ([]PostalRow, error) {
	f, err := openFile(seedFS, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParsePostal(f, country)
}

func readOrgs(seedFS fs.FS, path string) ([]OrgEntry, error) {
	f, err := openFile(seedFS, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseOrgs(f)
}
