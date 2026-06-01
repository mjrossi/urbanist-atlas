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

// regionsFile is the root of a regions_<cc>.toml document. The TOML
// schema is a single `region` array of tables, each unmarshaling
// directly into atlas.Region via the toml tags on that type. ID and
// Country come in zero from the parse and are stamped by the
// BuildMemStore caller.
type regionsFile struct {
	Regions []atlas.Region `toml:"region"`
}

// ParseRegions decodes one regions_<cc>.toml document and validates
// it structurally. ID and Country come in zero on each row; the
// caller stamps them.
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

// DetectCycles walks the staged region graph via DFS with 3-coloring
// (white/gray/black). Parents not defined in this slice are allowed
// and skipped during the walk — they're assumed to exist in an
// already-loaded file (cross-file parent edges from leaves up into
// the state tier are the canonical use case).
//
// This is the per-file FAST EARLY SIGNAL: it catches a cycle wholly
// contained in one file at the moment that file parses, before the
// rest of the bundle loads. It deliberately skips cross-file parent
// edges (those are validated globally after Stage 1 by
// DetectCyclesGraph over the fully-assembled graph — see build.go).
func DetectCycles(rs []atlas.Region) error {
	parents := map[string][]string{}
	inFile := map[string]bool{}
	for _, r := range rs {
		parents[r.Slug] = r.ParentSlugs
		inFile[r.Slug] = true
	}

	// Skip cross-file parents: they resolve in an already-loaded file
	// and are covered by the global DetectCyclesGraph pass.
	keep := func(slug, parent string) bool { return inFile[parent] }
	return detectCycles3Color(slugs(rs), parents, keep)
}

// DetectCyclesGraph runs the IDENTICAL 3-color DFS as DetectCycles but
// over the fully-assembled parents map (slug -> parent slugs across
// ALL seed files), with NO cross-file skip. It is BuildMemStore's
// REDUNDANT global acyclicity backstop, run after Stage 1. NOTE: under
// BuildMemStore this is defense-in-depth, not the primary proof — the
// load-order unknown-parent guard (build.go :107) already forces every
// parent edge to point backward in registration order, which is acyclic
// by construction, so a would-be cross-file back-edge is rejected there
// before this DFS ever sees a cyclic graph. This pass becomes the real
// backstop only if that guard is loosened to allow forward references.
// It is also exported (alongside DetectCycles) for callers that assemble
// a parents map directly, for which it is a genuine cycle detector.
func DetectCyclesGraph(parents map[string][]string) error {
	all := make([]string, 0, len(parents))
	for slug := range parents {
		all = append(all, slug)
	}
	// Sort so the walk order — and therefore the reported cycle path —
	// is deterministic across runs.
	slices.Sort(all)
	keep := func(slug, parent string) bool { return true }
	return detectCycles3Color(all, parents, keep)
}

// detectCycles3Color is the shared 3-color (white/gray/black) DFS body.
// order is the deterministic slug iteration order; parents maps a slug
// to its parent slugs; keep reports whether a given (slug, parent)
// edge should be walked (the per-file variant skips cross-file edges,
// the global variant walks everything).
func detectCycles3Color(order []string, parents map[string][]string, keep func(slug, parent string) bool) error {
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
			if !keep(slug, p) {
				continue
			}
			// Copy the path on descent so each branch owns its slice. A
			// bare append(path, slug) reuses path's backing array when
			// capacity allows, letting sibling parent edges in this loop
			// overwrite each other's path — which would interleave slugs
			// from a sibling branch into the reported cycle path on a
			// multi-parent (diamond) graph. Detection itself is carried by
			// the color map and is unaffected; this only protects the
			// human-readable error path.
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

// slugs returns the slug of each region, preserving order.
func slugs(rs []atlas.Region) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Slug
	}
	return out
}
