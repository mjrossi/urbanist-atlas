package seedfiles

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// regionsFile is the root of a regions_*.toml document: one `region`
// array of tables decoded straight into atlas.Region.
type regionsFile struct {
	Regions []atlas.Region `toml:"region"`
}

// ParseRegions decodes and structurally validates one regions_*.toml
// document. ID and Country are left zero for the caller to stamp.
func ParseRegions(r io.Reader) ([]atlas.Region, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("seedfiles: read regions: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("seedfiles: empty regions file")
	}
	var f regionsFile
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("seedfiles: parse regions toml: %w", err)
	}
	if err := validateRegions(f.Regions); err != nil {
		return nil, err
	}
	return f.Regions, nil
}

func validateRegions(rs []atlas.Region) error {
	if len(rs) == 0 {
		return errors.New("seedfiles: no regions in file")
	}
	seen := map[string]bool{}
	for i, r := range rs {
		ctx := fmt.Sprintf("region[%d] (slug=%q)", i, r.Slug)
		if r.Slug == "" {
			return fmt.Errorf("%s: slug required", ctx)
		}
		if seen[r.Slug] {
			return fmt.Errorf("%s: duplicate slug", ctx)
		}
		seen[r.Slug] = true
		if r.Kind == "" {
			return fmt.Errorf("%s: kind required", ctx)
		}
		if r.Name == "" {
			return fmt.Errorf("%s: name required", ctx)
		}
		switch r.ScopeTier {
		case atlas.ScopeLocal, atlas.ScopeRegional, atlas.ScopeNational:
		default:
			return fmt.Errorf("%s: scope_tier must be 'local', 'regional', or 'national' (got %q)", ctx, r.ScopeTier)
		}
		if r.SortPriority < 0 {
			return fmt.Errorf("%s: sort_priority must be non-negative", ctx)
		}
	}
	return nil
}

// DetectCycles reports a parent cycle wholly inside one file, skipping
// parents defined in other files. It is the per-file early signal;
// DetectCyclesGraph checks the assembled graph.
func DetectCycles(rs []atlas.Region) error {
	parents := map[string][]string{}
	inFile := map[string]bool{}
	order := make([]string, len(rs))
	for i, r := range rs {
		parents[r.Slug] = r.ParentSlugs
		inFile[r.Slug] = true
		order[i] = r.Slug
	}

	keep := func(parent string) bool { return inFile[parent] }
	return detectCycles3Color(order, parents, keep)
}

// DetectCyclesGraph runs the same DFS as DetectCycles over a full slug →
// parent slugs map, walking every edge.
func DetectCyclesGraph(parents map[string][]string) error {
	all := make([]string, 0, len(parents))
	for slug := range parents {
		all = append(all, slug)
	}
	// Sorted so the reported cycle path is deterministic.
	slices.Sort(all)
	keep := func(parent string) bool { return true }
	return detectCycles3Color(all, parents, keep)
}

// detectCycles3Color is the shared white/gray/black DFS. keep reports
// whether a parent edge is walked.
func detectCycles3Color(order []string, parents map[string][]string, keep func(parent string) bool) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}

	var dfs func(slug string, path []string) error
	dfs = func(slug string, path []string) error {
		switch color[slug] {
		case black:
			return nil
		case gray:
			return fmt.Errorf("seedfiles: cycle detected in parent graph:\n  %s\nfix the parents field on one of these regions", strings.Join(append(path, slug), " → "))
		}
		color[slug] = gray
		for _, p := range parents[slug] {
			if !keep(p) {
				continue
			}
			// Copy the path so sibling branches of a diamond never share a
			// backing array. The current first-cycle report is correct either
			// way (TestDetectCyclesGraph_DiamondCyclePathFidelity); the copy
			// guards any change that keeps paths around.
			child := append(append([]string(nil), path...), slug)
			if err := dfs(p, child); err != nil {
				return err
			}
		}
		color[slug] = black
		return nil
	}
	for _, slug := range order {
		if err := dfs(slug, nil); err != nil {
			return err
		}
	}
	return nil
}
