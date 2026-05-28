package seedfiles

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/pelletier/go-toml/v2"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// OrgEntry is one [[org]] in orgs.toml. It embeds atlas.Org for the
// shared wire-shape fields and adds the load-time RegionSlugs list
// — the latter is the only field that doesn't have a home on the
// runtime Org type (Org.Regions carries hydrated Region structs, not
// slugs).
//
// go-toml/v2 walks anonymous embedded fields the same way it walks
// inline ones, so the toml tags on atlas.Org's exported fields are
// honored at the outer-table level.
type OrgEntry struct {
	atlas.Org
	RegionSlugs []string `toml:"region_slugs"`
}

type orgsFile struct {
	Orgs []OrgEntry `toml:"org"`
}

// ParseOrgs decodes orgs.toml and runs structural validation. The
// returned slice has each entry's atlas.Org fields populated from
// TOML plus the load-only RegionSlugs list.
func ParseOrgs(r io.Reader) ([]OrgEntry, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("seedfiles: read orgs: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("seedfiles: empty orgs file")
	}
	var f orgsFile
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("seedfiles: parse orgs toml: %w", err)
	}
	if err := validateOrgs(f.Orgs); err != nil {
		return nil, err
	}
	return f.Orgs, nil
}

func validateOrgs(os []OrgEntry) error {
	if len(os) == 0 {
		return errors.New("seedfiles: no orgs in file")
	}
	seen := map[string]bool{}
	for i, o := range os {
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
