// Package seed parses the hand-curated organizations dataset
// (api/seed/orgs.toml).
//
// Each [[org]] entry creates one record with region_slugs that
// reference regions already loaded from the region TOML files. The
// parser is pure (no persistence); the FileStore loader in
// internal/loaddata composes Parse with the region-graph parsers to
// build the runtime atlas.MemStore.
package seed

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/pelletier/go-toml/v2"
)

// File is the root of orgs.toml.
type File struct {
	Orgs []Org `toml:"org"`
}

// Org is one [[org]] entry. Mirrors the wire/storage shape; the
// FileStore loader resolves RegionSlugs to in-memory region IDs.
type Org struct {
	Slug        string   `toml:"slug"`
	Name        string   `toml:"name"`
	ShortDesc   string   `toml:"short_desc"`
	WebsiteURL  string   `toml:"website_url"`
	ContactURL  string   `toml:"contact_url,omitempty"`
	Tags        []string `toml:"tags"`
	RegionSlugs []string `toml:"region_slugs"`
}

// Parse decodes orgs.toml from r and runs structural validation.
func Parse(r io.Reader) (File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return File{}, fmt.Errorf("seed: read: %w", err)
	}
	if len(data) == 0 {
		return File{}, errors.New("seed: empty file")
	}
	var f File
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("seed: parse toml: %w", err)
	}
	if err := validate(f); err != nil {
		return File{}, err
	}
	return f, nil
}

func validate(f File) error {
	if len(f.Orgs) == 0 {
		return errors.New("seed: no orgs in file")
	}
	seen := map[string]bool{}
	for i, o := range f.Orgs {
		ctx := fmt.Sprintf("orgs[%d] (slug=%q)", i, o.Slug)
		if o.Slug == "" {
			return fmt.Errorf("%s: slug required", ctx)
		}
		if seen[o.Slug] {
			return fmt.Errorf("%s: duplicate slug", ctx)
		}
		seen[o.Slug] = true
		if o.Name == "" {
			return fmt.Errorf("%s: name required", ctx)
		}
		if o.ShortDesc == "" {
			return fmt.Errorf("%s: short_desc required", ctx)
		}
		if o.WebsiteURL == "" {
			return fmt.Errorf("%s: website_url required", ctx)
		}
		if len(o.RegionSlugs) == 0 {
			return fmt.Errorf("%s: region_slugs must have at least one entry", ctx)
		}
	}
	return nil
}
