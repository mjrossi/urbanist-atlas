package loadregions

import (
	"fmt"
	"strings"
)

// DetectCycles validates the staged region graph in two passes:
//
//  1. every parent slug must be defined in the file (no dangling
//     references; parent regions from another country's file would
//     need their own loadregions run first — we don't cross files);
//  2. DFS with 3-coloring (white/gray/black) catches any cycle and
//     returns a human-readable trace.
func DetectCycles(f File) error {
	defined := map[string]bool{}
	for _, r := range f.Regions {
		defined[r.Slug] = true
	}
	for _, r := range f.Regions {
		for _, p := range r.Parents {
			if !defined[p] {
				return fmt.Errorf("loadregions: region %q lists unknown parent %q; declare %q in this file or remove the reference", r.Slug, p, p)
			}
		}
	}

	parents := map[string][]string{}
	for _, r := range f.Regions {
		parents[r.Slug] = r.Parents
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
			return fmt.Errorf("loadregions: cycle detected in parent graph:\n  %s\nfix the parents: field on one of these regions.", strings.Join(append(path, slug), " → "))
		}
		color[slug] = gray
		for _, p := range parents[slug] {
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
