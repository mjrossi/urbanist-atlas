package seedfiles

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// OrgEntry is one [[org]] in orgs.toml: atlas.Org plus the load-time
// fields atlas.Org can't hold. RegionSlugs has no home there (Org.Regions
// holds hydrated regions), and AddedAt shadows Org.AddedAt, which is
// toml:"-", to read the date-only value.
type OrgEntry struct {
	atlas.Org
	RegionSlugs []string       `toml:"region_slugs"`
	AddedAt     toml.LocalDate `toml:"added_at"`
}

type orgsFile struct {
	Orgs []OrgEntry `toml:"org"`
}

// ParseOrgs decodes orgs.toml and runs structural validation.
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
		if o.Slug != strings.ToLower(o.Slug) {
			return fmt.Errorf("%s: slug must be lowercase", ctx)
		}
		if seen[o.Slug] {
			return fmt.Errorf("%s: duplicate slug", ctx)
		}
		seen[o.Slug] = true
		tagStrs := make([]string, len(o.Tags))
		for i, t := range o.Tags {
			tagStrs[i] = string(t)
		}
		if err := ValidateOrgFields(o.Name, o.ShortDesc, o.WebsiteURL, o.ContactURL, tagStrs, o.RegionSlugs); err != nil {
			return fmt.Errorf("%s: %w", ctx, err)
		}
	}
	return nil
}

// ValidateOrgFields runs the per-record field checks for one orgs.toml
// entry. Slug uniqueness needs every row, so validateOrgs checks it.
func ValidateOrgFields(name, shortDesc, websiteURL, contactURL string, tags, regionSlugs []string) error {
	if name == "" {
		return errors.New("name required")
	}
	if utf8.RuneCountInString(name) > atlas.MaxNameLen {
		return fmt.Errorf("name must be at most %d characters", atlas.MaxNameLen)
	}
	if shortDesc == "" {
		return errors.New("short_desc required")
	}
	if utf8.RuneCountInString(shortDesc) > atlas.MaxShortDescLen {
		return fmt.Errorf("short_desc must be at most %d characters", atlas.MaxShortDescLen)
	}
	if websiteURL == "" {
		return errors.New("website_url required")
	}
	if err := validateHTTPURL("website_url", websiteURL); err != nil {
		return err
	}
	if contactURL != "" {
		if err := validateContactURL(contactURL); err != nil {
			return err
		}
	}
	if len(tags) > atlas.MaxTags {
		return fmt.Errorf("tags must have at most %d entries", atlas.MaxTags)
	}
	for i, tag := range tags {
		if tag == "" {
			return fmt.Errorf("tags[%d] empty", i)
		}
		if utf8.RuneCountInString(tag) > atlas.MaxTagLen {
			return fmt.Errorf("tags[%d] must be at most %d characters", i, atlas.MaxTagLen)
		}
	}
	if len(regionSlugs) == 0 {
		return errors.New("region_slugs must have at least one entry")
	}
	if len(regionSlugs) > atlas.MaxRegionSlugs {
		return fmt.Errorf("region_slugs must have at most %d entries", atlas.MaxRegionSlugs)
	}
	for i, slug := range regionSlugs {
		if slug == "" {
			return fmt.Errorf("region_slugs[%d] empty", i)
		}
		if len(slug) > atlas.MaxRegionSlugLen {
			return fmt.Errorf("region_slugs[%d] must be at most %d characters", i, atlas.MaxRegionSlugLen)
		}
	}
	return nil
}

// validateHTTPURL accepts only absolute http(s) URLs with a host.
func validateHTTPURL(field, raw string) error {
	reason, perr := atlas.ClassifyURL(raw, false)
	switch reason {
	case atlas.URLOK:
		return nil
	case atlas.URLTooLong:
		return fmt.Errorf("%s must be at most %d characters", field, atlas.MaxURLLen)
	case atlas.URLParseFailed:
		return fmt.Errorf("%s is not a valid URL: %w", field, perr)
	case atlas.URLBadScheme:
		return fmt.Errorf("%s must use http or https", field)
	case atlas.URLMissingHost:
		return fmt.Errorf("%s must include a host", field)
	default:
		return fmt.Errorf("%s must use http or https", field)
	}
}

// validateContactURL also accepts mailto: with an address, since some
// orgs publish only a mailbox.
func validateContactURL(raw string) error {
	reason, perr := atlas.ClassifyURL(raw, true)
	switch reason {
	case atlas.URLOK:
		return nil
	case atlas.URLTooLong:
		return fmt.Errorf("contact_url must be at most %d characters", atlas.MaxURLLen)
	case atlas.URLParseFailed:
		return fmt.Errorf("contact_url is not a valid URL: %w", perr)
	case atlas.URLMissingHost:
		return errors.New("contact_url must include a host")
	case atlas.URLMailtoNoAddress:
		return errors.New("contact_url mailto: must include an address")
	default: // atlas.URLBadScheme
		return errors.New("contact_url must use http, https, or mailto")
	}
}
