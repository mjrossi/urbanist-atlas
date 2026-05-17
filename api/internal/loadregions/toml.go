// Package loadregions reads region taxonomy TOML files (regions_<cc>.toml)
// and writes the regions + region_parents rows inside a single
// transaction. Cycle detection happens at staging time before any DB
// write occurs.
//
// Schema reference: docs/region-graph.md.
package loadregions

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/pelletier/go-toml/v2"
)

// File is the root of a regions_<cc>.toml document.
type File struct {
	Regions []Region `toml:"region"`
}

// Region mirrors the wire/storage Region shape, with Parents as a list
// of slugs (resolved to IDs at write time).
type Region struct {
	Slug         string   `toml:"slug"`
	Kind         string   `toml:"kind"`
	Name         string   `toml:"name"`
	ScopeTier    string   `toml:"scope_tier"`
	SortPriority int      `toml:"sort_priority"`
	Parents      []string `toml:"parents"`
}

// Parse decodes a regions TOML document from r and runs structural
// validation: required fields present, scope_tier is local|regional,
// no duplicate slugs. Cycle detection is in validate.go.
func Parse(r io.Reader) (File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return File{}, fmt.Errorf("loadregions: read: %w", err)
	}
	if len(data) == 0 {
		return File{}, errors.New("loadregions: empty file")
	}
	var f File
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("loadregions: parse toml: %w", err)
	}
	if err := validateStructural(f); err != nil {
		return File{}, err
	}
	return f, nil
}

func validateStructural(f File) error {
	if len(f.Regions) == 0 {
		return errors.New("loadregions: no regions in file")
	}
	seen := map[string]bool{}
	for i, r := range f.Regions {
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
		if r.ScopeTier != "local" && r.ScopeTier != "regional" {
			return fmt.Errorf("%s: scope_tier must be 'local' or 'regional' (got %q)", ctx, r.ScopeTier)
		}
		if r.SortPriority < 0 {
			return fmt.Errorf("%s: sort_priority must be non-negative", ctx)
		}
	}
	return nil
}
