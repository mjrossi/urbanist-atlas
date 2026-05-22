package loadregions

import (
	"fmt"
	"strings"
)

// DetectCycles checks the staged region graph for cycles via DFS with
// 3-coloring (white/gray/black). Parents not defined in this file are
// allowed and skipped during the walk — they're assumed to exist in
// the DB (resolved at write time via RegionIDBySlug). Splitting region
// data across multiple files (e.g., regions_us_states.toml loads the
// state tier first; regions_us.toml's leaves then parent under those
// states) is the canonical use case.
//
// Cross-file cycle detection (considering DB-resident edges as well)
// is intentionally out of scope: the only cross-file parent edges
// in practice point from leaves up into the state tier, and state-tier
// files have no parents — so they can't introduce cycles when loaded.
// If cross-file cycle scenarios become real, extend this pass to
// query the DB for parents of regions whose slugs appear in our file.
func DetectCycles(f File) error {
	parents := map[string][]string{}
	inFile := map[string]bool{}
	for _, r := range f.Regions {
		parents[r.Slug] = r.Parents
		inFile[r.Slug] = true
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	for _, r := range f.Regions {
		color[r.Slug] = white
	}

	var dfs func(slug string, path []string) error
	dfs = func(slug string, path []string) error {
		switch color[slug] {
		case black:
			return nil
		case gray:
			return fmt.Errorf("loadregions: cycle detected in parent graph:\n  %s\nfix the parents: field on one of these regions", strings.Join(append(path, slug), " → "))
		}
		color[slug] = gray
		for _, p := range parents[slug] {
			if !inFile[p] {
				continue // cross-file parent, resolved at write time
			}
			if err := dfs(p, append(path, slug)); err != nil {
				return err
			}
		}
		color[slug] = black
		return nil
	}
	for _, r := range f.Regions {
		if err := dfs(r.Slug, nil); err != nil {
			return err
		}
	}
	return nil
}
