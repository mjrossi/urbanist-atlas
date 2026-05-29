package seedfiles

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Field length caps shared by the seed loader and the submissions
// handler. Generous next to current seed data (longest name is ~46
// chars, longest short_desc ~383) but tight enough that a malicious
// submission can't store megabytes of JSON or render an unreadable
// orgs.toml entry. ValidateOrgFields enforces both the bounds and the
// URL/host shape; callers don't need to pre-check.
const (
	MaxNameLen       = 200
	MaxShortDescLen  = 1000
	MaxURLLen        = 500
	MaxTagLen        = 50
	MaxTags          = 20
	MaxRegionSlugLen = 100
	MaxRegionSlugs   = 20
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

// ValidateOrgFields runs the per-entry field checks shared between the
// seed-file loader and the public submissions handler. It enforces
// presence, length caps, URL shape (absolute http/https with host),
// and per-element tag/slug bounds.
//
// The slug/uniqueness checks live in validateOrgs because they're
// only meaningful in a multi-row context; submissions don't carry a
// slug at all (moderators assign one when approving). The submitter
// fields are handler-only and validated separately.
func ValidateOrgFields(name, shortDesc, websiteURL, contactURL string, tags, regionSlugs []string) error {
	if name == "" {
		return errors.New("name required")
	}
	if utf8.RuneCountInString(name) > MaxNameLen {
		return fmt.Errorf("name must be at most %d characters", MaxNameLen)
	}
	if shortDesc == "" {
		return errors.New("short_desc required")
	}
	if utf8.RuneCountInString(shortDesc) > MaxShortDescLen {
		return fmt.Errorf("short_desc must be at most %d characters", MaxShortDescLen)
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
	if len(tags) > MaxTags {
		return fmt.Errorf("tags must have at most %d entries", MaxTags)
	}
	for i, tag := range tags {
		if tag == "" {
			return fmt.Errorf("tags[%d] empty", i)
		}
		if utf8.RuneCountInString(tag) > MaxTagLen {
			return fmt.Errorf("tags[%d] must be at most %d characters", i, MaxTagLen)
		}
	}
	if len(regionSlugs) == 0 {
		return errors.New("region_slugs must have at least one entry")
	}
	if len(regionSlugs) > MaxRegionSlugs {
		return fmt.Errorf("region_slugs must have at most %d entries", MaxRegionSlugs)
	}
	for i, slug := range regionSlugs {
		if slug == "" {
			return fmt.Errorf("region_slugs[%d] empty", i)
		}
		if len(slug) > MaxRegionSlugLen {
			return fmt.Errorf("region_slugs[%d] must be at most %d characters", i, MaxRegionSlugLen)
		}
	}
	return nil
}

// validateHTTPURL accepts only absolute http(s) URLs with a host. Used
// for website_url. The length cap is generous (500 chars) but caps
// the worst case so an attacker can't store megabytes of nonsense in
// orgs.toml.
func validateHTTPURL(field, raw string) error {
	if len(raw) > MaxURLLen {
		return fmt.Errorf("%s must be at most %d characters", field, MaxURLLen)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL: %w", field, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%s must use http or https", field)
	}
	if u.Host == "" {
		return fmt.Errorf("%s must include a host", field)
	}
	return nil
}

// validateContactURL is contact_url's looser cousin: http(s) for
// online contact forms, mailto: for direct email (a few seed orgs
// expose only a mailbox). mailto must include an address; opaque
// schemes without one ("mailto:" alone) are rejected.
func validateContactURL(raw string) error {
	if len(raw) > MaxURLLen {
		return fmt.Errorf("contact_url must be at most %d characters", MaxURLLen)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("contact_url is not a valid URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return errors.New("contact_url must include a host")
		}
		return nil
	case "mailto":
		if u.Opaque == "" {
			return errors.New("contact_url mailto: must include an address")
		}
		return nil
	default:
		return errors.New("contact_url must use http, https, or mailto")
	}
}
