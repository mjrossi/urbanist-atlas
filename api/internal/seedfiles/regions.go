package seedfiles

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
// Cross-file cycle detection is intentionally out of scope: the only
// cross-file parent edges in practice point from leaves up into a
// state/province tier whose files have no parents themselves, so
// they can't introduce cycles when loaded in order.
func DetectCycles(rs []atlas.Region) error {
	parents := map[string][]string{}
	inFile := map[string]bool{}
	for _, r := range rs {
		parents[r.Slug] = r.ParentSlugs
		inFile[r.Slug] = true
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	for _, r := range rs {
		color[r.Slug] = white
	}

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
			if !inFile[p] {
				continue // cross-file parent, resolved at load time
			}
			if err := dfs(p, append(path, slug)); err != nil {
				return err
			}
		}
		color[slug] = black
		return nil
	}
	for _, r := range rs {
		if err := dfs(r.Slug, nil); err != nil {
			return err
		}
	}
	return nil
}
