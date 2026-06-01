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

// countrySpec describes one country's bundled seed files.
//
//   - Code:        canonical upper-case country code stamped on every region.
//   - RegionFiles: file suffixes for regions_<suffix>.toml, in load
//     order. Earlier files load first; later files may reference
//     earlier-loaded regions as parents.
//   - Postal:      file suffix for postal_codes_<suffix>.csv.
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
		out = append(out, c.Code)
	}
	return out
}

// BuildMemStore parses every bundled seed file from seedFS and
// returns a populated atlas.MemStore. Synthetic int64 IDs are
// assigned to regions and orgs in load order; the wire contract
// identifies both by slug, so the IDs are stable-within-process and
// process-only.
func BuildMemStore(logger *slog.Logger, seedFS fs.FS) (*atlas.MemStore, error) {
	return buildMemStore(logger, seedFS, countries)
}

// buildMemStore is the BuildMemStore core, parameterized on the
// country set so tests can drive the loader (and its global invariant
// checks) over a tiny synthetic bundle. Production callers go through
// BuildMemStore, which passes the package-level countries.
func buildMemStore(logger *slog.Logger, seedFS fs.FS, countrySet []countrySpec) (*atlas.MemStore, error) {
	store := atlas.NewMemStore()
	regionIDBySlug := map[string]int64{}
	// parentSlugs is the assembled cross-file parent graph (slug ->
	// parent slugs across ALL files); childCount counts direct children
	// per slug so the orphan check can tell leaves from interior nodes;
	// localTier marks the local-tier (city/neighborhood) leaves the
	// RGN-02b coverage check applies to (see assertReachableLeaves).
	parentSlugs := map[string][]string{}
	childCount := map[string]int{}
	localTier := map[string]bool{}
	var nextRegionID int64 = 1

	// Stage 1: regions, in dependency order.
	for _, c := range countrySet {
		for _, suffix := range c.RegionFiles {
			path := "regions_" + suffix + ".toml"
			regions, err := readRegions(seedFS, path)
			if err != nil {
				return nil, fmt.Errorf("seedfiles: regions %s/%s: %w", c.Code, suffix, err)
			}
			// Fast early signal: a cycle wholly inside this file fails
			// now, before the rest of the bundle loads. The global
			// cross-file check runs after the loop (RGN-02a).
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
				store.AddRegion(r)
			}
		}
	}

	// RGN-02a: redundant global acyclicity proof over the
	// FULLY-ASSEMBLED parent graph. NOTE: the load-order guard above
	// (every parent slug must already be registered before a region's
	// own slug — build.go :107) is what actually guarantees acyclicity.
	// It forces every parent edge to point backward in registration
	// order, and a graph whose edges all respect a single total order is
	// acyclic by construction — so this DFS cannot fire on any input
	// BuildMemStore accepts (a cross-file back-edge is rejected earlier
	// at :107 with "references unknown parent slug"). It is retained as
	// defense-in-depth, NOT as the primary proof: if the unknown-parent
	// guard is ever loosened to permit forward references / out-of-order
	// files, this DFS becomes the real backstop against an infinite
	// ancestor/descendant walk at runtime. Do not weaken :107 on the
	// assumption that this check is the cycle backstop — it only fires
	// once :107 stops forcing backward edges.
	if err := DetectCyclesGraph(parentSlugs); err != nil {
		return nil, fmt.Errorf("seedfiles: global region graph: %w", err)
	}

	// Stage 2: postal codes, country-by-country. anchoredSlugs records
	// every slug a postal row points at (its leaf anchor) for the
	// RGN-02b reachability check.
	anchoredSlugs := map[string]bool{}
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
			store.AddPostalCode(row.Country, row.PostalCode, leafID)
		}
		if logger != nil {
			logger.Debug("seedfiles: postal loaded", "country", c.Code, "rows", len(rows))
		}
	}

	// Stage 3: orgs. orgSlugs records every slug an org attaches to for
	// the RGN-02b reachability check — but ONLY slugs that resolved.
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
		// added_at is required; a missing TOML key parses as the zero
		// toml.LocalDate (Year == 0). Reject loudly with the offending
		// slug so an operator can fix the seed file without grep.
		if entry.AddedAt.Year == 0 {
			return nil, fmt.Errorf("seedfiles: org %q: missing required added_at", entry.Slug)
		}
		// INVARIANT: only an org slug that resolved to a region ID may
		// anchor a leaf for the RGN-02b reachability check. resolveOrgRegions
		// above hard-errors on any slug missing from regionIDBySlug, so by
		// the time we reach here every slug in entry.RegionSlugs is known to
		// resolve. Keep this loop AFTER the resolve check (do not hoist it):
		// recording an unresolved slug would let a typo'd org slug mark a
		// real, differently-owned leaf as "anchoring" and suppress a genuine
		// orphan-leaf error for it.
		for _, slug := range entry.RegionSlugs {
			orgSlugs[slug] = true
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

	// RGN-02b: prove region->postal reachability. Stage 2 already
	// proved postal->region (every postal row's leaf slug resolves);
	// this asserts the inverse — every LOCAL-tier LEAF region (no
	// children) is reachable via a postal anchor, an attached org, or
	// an anchoring descendant. A local leaf is the coverage-bearing
	// city/neighborhood node; if an ETL change drops its postal rows it
	// orphans silently — invisible in browse, unreachable by /lookup —
	// hiding a coverage regression behind a clean boot. Fail closed so
	// the gap surfaces here.
	if err := assertReachableLeaves(localTier, childCount, anchoredSlugs, orgSlugs); err != nil {
		return nil, err
	}

	if logger != nil {
		logger.Info("seedfiles: filestore built",
			"regions", len(regionIDBySlug),
			"orgs", len(orgs),
		)
	}
	return store, nil
}

// assertReachableLeaves enforces the RGN-02b invariant over the
// LOCAL-tier leaves: every local-tier region with no children in the
// assembled graph must have ≥1 postal anchor, ≥1 attached org, or an
// anchoring descendant. A leaf by definition has no descendants, so
// the descendant leg is vacuous for it and the check reduces to
// "anchored by a postal row or an org". Interior (non-leaf) regions
// are anchored transitively by their descendants and are not checked.
//
// Scope = local tier deliberately. The finding (RGN-02b) targets the
// coverage-bearing city/neighborhood leaves whose silent loss of
// postal rows would hide a coverage regression. Regional-tier leaves
// with no postal anchor are the editorially-known coarse-coverage
// condition (e.g. the CA CMAs whose finer postal data is PCCF-pending,
// deferred under ETL-03) — a deliberate fallback to the province, not
// an orphan, so they are out of scope here. The slug list is sorted and
// the full count is reported (with a capped sample) so a bulk ETL change
// that orphans many leaves at once surfaces all of them in one run,
// deterministically across runs.
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
	// Report all orphans (capped sample) so a bulk regression that
	// orphans N leaves surfaces in one build instead of forcing N
	// fix-rerun cycles. cap is a builtin, so the limit is maxShown.
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
