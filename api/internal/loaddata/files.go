package loaddata

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/mjrossi/urbanist-atlas/api/internal/loadpostal"
	"github.com/mjrossi/urbanist-atlas/api/internal/loadregions"
	"github.com/mjrossi/urbanist-atlas/api/internal/seed"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// BuildMemStore parses every bundled seed file under seedDir and
// returns a populated atlas.MemStore. The load order matches LoadAll:
// regions (in the dependency order recorded in `countries`), then
// postal codes, then orgs.
//
// Synthetic int64 IDs are assigned to regions and orgs in load order.
// IDs are stable within a single process but not across processes —
// the wire contract identifies both by slug, so this is fine.
//
// Returns an error if any file is missing, parses invalidly, or
// references a slug that hasn't been loaded yet. Unknown postal-code
// leaf slugs and unknown org region slugs are hard errors (same as
// the Postgres-write path).
func BuildMemStore(logger *slog.Logger, seedDir string) (*atlas.MemStore, error) {
	store := atlas.NewMemStore()

	// Region slugs collected as we add them so postal + orgs can
	// resolve cross-file references without re-reading files.
	regionIDBySlug := map[string]int64{}
	var nextRegionID int64 = 1

	// Stage 1: regions, in dependency order.
	for _, c := range countries {
		for _, suffix := range c.regionFiles {
			path := filepath.Join(seedDir, "regions_"+suffix+".toml")
			f, err := openFile(path)
			if err != nil {
				return nil, fmt.Errorf("loaddata: regions %s/%s: %w", c.code, suffix, err)
			}
			file, err := loadregions.Parse(f)
			_ = f.Close()
			if err != nil {
				return nil, fmt.Errorf("loaddata: regions %s/%s: %w", c.code, suffix, err)
			}
			if err := loadregions.DetectCycles(file); err != nil {
				return nil, fmt.Errorf("loaddata: regions %s/%s: %w", c.code, suffix, err)
			}
			// Add regions in file order. Parents may live in earlier-
			// loaded files (cross-file resolution); MemStore.AddRegion
			// looks up parent slugs against its own index so anything
			// already registered resolves.
			for _, r := range file.Regions {
				if _, dup := regionIDBySlug[r.Slug]; dup {
					return nil, fmt.Errorf("loaddata: duplicate region slug %q across files", r.Slug)
				}
				id := nextRegionID
				nextRegionID++
				regionIDBySlug[r.Slug] = id

				// Verify every parent slug exists at this point. If a
				// downstream file referenced an undefined parent, this
				// catches it where the file was loaded.
				for _, ps := range r.Parents {
					if _, ok := regionIDBySlug[ps]; !ok {
						return nil, fmt.Errorf("loaddata: region %q references unknown parent slug %q (load the defining file first)", r.Slug, ps)
					}
				}

				store.AddRegion(atlas.Region{
					ID:           id,
					Kind:         atlas.RegionKind(r.Kind),
					Name:         r.Name,
					Slug:         r.Slug,
					Country:      atlas.Country(c.code),
					ScopeTier:    atlas.ScopeTier(r.ScopeTier),
					SortPriority: r.SortPriority,
					ParentSlugs:  append([]string(nil), r.Parents...),
				})
			}
		}
	}

	// Stage 2: postal codes, country-by-country.
	for _, c := range countries {
		path := filepath.Join(seedDir, "postal_codes_"+c.postal+".csv")
		f, err := openFile(path)
		if err != nil {
			return nil, fmt.Errorf("loaddata: postal %s: %w", c.code, err)
		}
		rows, err := loadpostal.ParseCSV(f, atlas.Country(c.code))
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("loaddata: postal %s: %w", c.code, err)
		}
		for _, row := range rows {
			leafID, ok := regionIDBySlug[row.LeafRegionSlug]
			if !ok {
				return nil, fmt.Errorf("loaddata: postal %s/%s: leaf_region_slug %q not found", row.Country, row.PostalCode, row.LeafRegionSlug)
			}
			store.AddPostalCode(row.Country, row.PostalCode, leafID)
		}
		if logger != nil {
			logger.Debug("loaddata: postal loaded", "country", c.code, "rows", len(rows))
		}
	}

	// Stage 3: orgs.
	orgsPath := filepath.Join(seedDir, "orgs.toml")
	f, err := openFile(orgsPath)
	if err != nil {
		return nil, fmt.Errorf("loaddata: orgs: %w", err)
	}
	orgFile, err := seed.Parse(f)
	_ = f.Close()
	if err != nil {
		return nil, fmt.Errorf("loaddata: orgs: %w", err)
	}
	var nextOrgID int64 = 1
	for _, o := range orgFile.Orgs {
		ids, err := resolveOrgRegions(o, regionIDBySlug)
		if err != nil {
			return nil, fmt.Errorf("loaddata: orgs: %w", err)
		}
		tags := make([]atlas.Tag, 0, len(o.Tags))
		for _, t := range o.Tags {
			tags = append(tags, atlas.Tag(t))
		}
		store.AddOrg(atlas.Org{
			ID:         nextOrgID,
			Slug:       o.Slug,
			Name:       o.Name,
			ShortDesc:  o.ShortDesc,
			WebsiteURL: o.WebsiteURL,
			ContactURL: o.ContactURL,
			Tags:       tags,
		}, ids)
		nextOrgID++
	}
	if logger != nil {
		logger.Info("loaddata: filestore built",
			"regions", len(regionIDBySlug),
			"orgs", len(orgFile.Orgs),
		)
	}
	return store, nil
}

// resolveOrgRegions returns the int64 region IDs for an org's
// region_slugs, sorted ascending so the wire shape matches the
// Postgres `ARRAY(... ORDER BY orx.region_id)` contract. Unknown
// slugs are a hard error.
func resolveOrgRegions(o seed.Org, regionIDBySlug map[string]int64) ([]int64, error) {
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

func openFile(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("seed file missing: %s", path)
		}
		return nil, err
	}
	return f, nil
}
