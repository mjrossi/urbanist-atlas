// Package seedfiles parses the bundled seed data (regions_*.toml,
// postal_codes_*.csv, orgs.toml) into an in-memory atlas.MemStore. It
// reads an fs.FS, so the same loader serves the production embed and a
// directory on disk.
//
// A new country needs its regions_<cc>*.toml and postal_codes_<cc>.csv
// in api/seed/ and an entry in countries, with any state-tier file
// listed first so the main file's leaves can parent under it.
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

// countrySpec names one country's seed files: RegionFiles are the
// regions_<suffix>.toml suffixes in load order (a region may only parent
// under regions loaded before it, from earlier files or earlier rows of
// the same file), Postal the postal_codes_<suffix>.csv suffix.
type countrySpec struct {
	Code        string
	RegionFiles []string
	Postal      string
}

// countries lists every country whose bundled seed gets loaded by
// BuildMemStore.
var countries = []countrySpec{
	{"US", []string{"us_states", "us_multistate", "us_msas", "us"}, "us"},
	{"CA", []string{"ca_provinces", "ca_cmas", "ca"}, "ca"},
	// PT's seed files stay in api/seed/ as an unused reference for a
	// future reactivation; nothing loads them.
}

// Countries returns the country codes BuildMemStore loads, in
// dependency order.
func Countries() []string {
	out := make([]string, 0, len(countries))
	for _, c := range countries {
		out = append(out, c.Code)
	}
	return out
}

// BuildMemStore parses every bundled seed file from seedFS into a
// MemStore. Region and org IDs are assigned in load order and are only
// stable within a process; the wire contract identifies both by slug.
func BuildMemStore(logger *slog.Logger, seedFS fs.FS) (*atlas.MemStore, error) {
	return buildMemStore(logger, seedFS, countries)
}

// buildMemStore takes the country set as a parameter so tests can run
// the loader and its global invariant checks over a tiny bundle.
func buildMemStore(logger *slog.Logger, seedFS fs.FS, countrySet []countrySpec) (*atlas.MemStore, error) {
	store := atlas.NewMemStore()
	regionIDBySlug := map[string]int64{}
	// childCount lets the orphan check tell leaves from interior nodes;
	// localTier marks the leaves it applies to.
	parentSlugs := map[string][]string{}
	childCount := map[string]int{}
	localTier := map[string]bool{}
	// rollup_states resolve after Stage 1 because a metro may name a
	// state from any loaded file, and they are not graph edges, so they
	// stay out of the load-order parent check.
	kindBySlug := map[string]atlas.RegionKind{}
	var rollupSources []atlas.Region
	var nextRegionID int64 = 1

	// Stage 1: regions, in dependency order.
	for _, c := range countrySet {
		for _, suffix := range c.RegionFiles {
			path := "regions_" + suffix + ".toml"
			regions, err := readRegions(seedFS, path)
			if err != nil {
				return nil, fmt.Errorf("seedfiles: regions %s/%s: %w", c.Code, suffix, err)
			}
			// Catch a cycle inside one file before the rest of the bundle loads.
			if err := DetectCycles(regions); err != nil {
				return nil, fmt.Errorf("seedfiles: regions %s/%s: %w", c.Code, suffix, err)
			}
			for _, r := range regions {
				if _, dup := regionIDBySlug[r.Slug]; dup {
					return nil, fmt.Errorf("seedfiles: duplicate region slug %q across files", r.Slug)
				}
				for _, ps := range r.ParentSlugs {
					if _, ok := regionIDBySlug[ps]; !ok {
						return nil, fmt.Errorf("seedfiles: region %q references unknown parent slug %q (load the defining file first)", r.Slug, ps)
					}
					childCount[ps]++
				}
				r.ID = nextRegionID
				r.Country = atlas.Country(c.Code)
				regionIDBySlug[r.Slug] = nextRegionID
				parentSlugs[r.Slug] = r.ParentSlugs
				if r.ScopeTier == atlas.ScopeLocal {
					localTier[r.Slug] = true
				}
				nextRegionID++
				if err := store.AddRegion(r); err != nil {
					return nil, fmt.Errorf("seedfiles: regions %s/%s: %w", c.Code, suffix, err)
				}
				kindBySlug[r.Slug] = r.Kind
				if len(r.RollupStates) > 0 {
					rollupSources = append(rollupSources, r)
				}
			}
		}
	}

	// Redundant: the unknown-parent check above forces every edge to point
	// backward in load order, which is acyclic by construction. This cycle
	// check is the backstop if the unknown-parent check is ever loosened to
	// allow forward references, so don't remove the cycle check on the
	// assumption that the parent check already catches cycles.
	if err := DetectCyclesGraph(parentSlugs); err != nil {
		return nil, fmt.Errorf("seedfiles: global region graph: %w", err)
	}

	// rollup_states never enter parentSlugs or the ancestor walk, which
	// keeps them leak-free (docs/region-graph.md §1). Each target must be a
	// state-equivalent or us:federal-district region.
	for _, r := range rollupSources {
		for _, target := range r.RollupStates {
			if _, ok := regionIDBySlug[target]; !ok {
				return nil, fmt.Errorf("seedfiles: region %q rollup_states references unknown slug %q", r.Slug, target)
			}
			if !atlas.IsRollupTargetKind(kindBySlug[target]) {
				return nil, fmt.Errorf("seedfiles: region %q rollup_states target %q must be a state-equivalent or us:federal-district kind (got %q)", r.Slug, target, kindBySlug[target])
			}
			store.AddRollupState(r.Slug, target)
		}
	}

	// Stage 2: postal codes. anchoredSlugs feeds the orphan-leaf check.
	anchoredSlugs := map[string]bool{}
	var postalTotal int
	for _, c := range countrySet {
		path := "postal_codes_" + c.Postal + ".csv"
		rows, err := readPostal(seedFS, path, atlas.Country(c.Code))
		if err != nil {
			return nil, fmt.Errorf("seedfiles: postal %s: %w", c.Code, err)
		}
		for _, row := range rows {
			leafID, ok := regionIDBySlug[row.LeafRegionSlug]
			if !ok {
				return nil, fmt.Errorf("seedfiles: postal %s/%s: leaf_region_slug %q not found", row.Country, row.PostalCode, row.LeafRegionSlug)
			}
			anchoredSlugs[row.LeafRegionSlug] = true
			if err := store.AddPostalCode(row.Country, row.PostalCode, leafID); err != nil {
				return nil, fmt.Errorf("seedfiles: postal %s: %w", c.Code, err)
			}
		}
		postalTotal += len(rows)
		if logger != nil {
			logger.Debug("seedfiles: postal loaded", "country", c.Code, "rows", len(rows))
		}
	}

	// Stage 3: orgs.
	orgs, err := readOrgs(seedFS, "orgs.toml")
	if err != nil {
		return nil, fmt.Errorf("seedfiles: orgs: %w", err)
	}
	orgSlugs := map[string]bool{}
	var nextOrgID int64 = 1
	for _, entry := range orgs {
		ids, err := resolveOrgRegions(entry, regionIDBySlug)
		if err != nil {
			return nil, fmt.Errorf("seedfiles: orgs: %w", err)
		}
		// A missing added_at parses as the zero toml.LocalDate.
		if entry.AddedAt.Year == 0 {
			return nil, fmt.Errorf("seedfiles: org %q: missing required added_at", entry.Slug)
		}
		// Record org slugs only after resolveOrgRegions has proved they all
		// resolve: an unresolved slug here could suppress a real orphan-leaf
		// error.
		for _, slug := range entry.RegionSlugs {
			orgSlugs[slug] = true
		}
		o := entry.Org
		o.ID = nextOrgID
		// added_at is date-only; pin it to midnight UTC.
		o.AddedAt = time.Date(entry.AddedAt.Year, time.Month(entry.AddedAt.Month), entry.AddedAt.Day, 0, 0, 0, 0, time.UTC)
		store.AddOrg(o, ids)
		nextOrgID++
	}

	// Every local-tier leaf must be reachable from a postal row or an org.
	// An ETL change that drops a city's postal rows would otherwise leave it
	// invisible in browse and unreachable by /lookup behind a clean boot.
	if err := assertReachableLeaves(localTier, childCount, anchoredSlugs, orgSlugs); err != nil {
		return nil, err
	}

	if logger != nil {
		logger.Info("seedfiles: filestore built",
			"regions", len(regionIDBySlug),
			"orgs", len(orgs),
			"postal_codes", postalTotal,
		)
	}
	return store, nil
}

// assertReachableLeaves fails when a local-tier region with no children
// has neither a postal anchor nor an attached org. Regional-tier leaves
// are exempt: falling back to a coarser anchor is an editorial choice
// there, not a lost city. The error lists every orphan, sorted and
// capped, so a bulk regression shows up in one run.
func assertReachableLeaves(localTier map[string]bool, childCount map[string]int, anchoredSlugs, orgSlugs map[string]bool) error {
	candidates := make([]string, 0, len(localTier))
	for slug := range localTier {
		if childCount[slug] > 0 {
			continue // interior node — anchored via descendants
		}
		if anchoredSlugs[slug] || orgSlugs[slug] {
			continue
		}
		candidates = append(candidates, slug)
	}
	if len(candidates) == 0 {
		return nil
	}
	slices.Sort(candidates)
	const maxShown = 20
	shown := candidates
	if len(shown) > maxShown {
		shown = shown[:maxShown]
	}
	return fmt.Errorf("seedfiles: %d orphan leaf region(s) with no postal anchor, "+
		"no attached org, and no anchoring descendant: %v "+
		"(add a postal row, attach an org, or remove the region)",
		len(candidates), shown)
}

// resolveOrgRegions returns an org's region IDs, deduplicated and sorted.
// An unknown slug is an error.
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

// openFile names the missing path when seedFS has no such file.
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
