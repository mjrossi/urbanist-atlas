package githubpr

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// RenderOrgBlock returns the `[[org]]` table block to append to
// api/seed/orgs.toml for an approved submission. The function is
// pure: same submission → byte-identical output. That matters
// because git diffs on the seed file are the editorial-review
// surface; consistent formatting keeps them signal-rich.
//
// Slug is derived from the submitted website hostname's last
// path-safe label; the maintainer can rewrite it during PR review
// without surprises (the slug isn't carried by the submission
// payload — see CLAUDE.md / spec for why moderators own that
// decision).
func RenderOrgBlock(sub atlas.Submission, slug string) (string, error) {
	if slug == "" {
		return "", fmt.Errorf("githubpr: empty slug for submission %s", sub.PublicID)
	}
	entry := orgTOMLEntry{
		Slug:        slug,
		Name:        sub.Payload.Name,
		ShortDesc:   sub.Payload.ShortDesc,
		WebsiteURL:  sub.Payload.WebsiteURL,
		ContactURL:  sub.Payload.ContactURL,
		Tags:        sub.Payload.Tags,
		RegionSlugs: sub.Payload.RegionSlugs,
	}
	doc := orgTOMLDoc{Org: []orgTOMLEntry{entry}}
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(false)
	if err := enc.Encode(doc); err != nil {
		return "", fmt.Errorf("githubpr: marshal org entry: %w", err)
	}
	return buf.String(), nil
}

// DeriveSlug returns a default slug from the submission's name.
// Lowercased, ASCII-letters/digits only, hyphen-joined. Moderators
// are free to rewrite it in the PR; this just produces a reasonable
// starting point.
func DeriveSlug(name string) string {
	var b strings.Builder
	prevHyphen := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case !prevHyphen:
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// orgTOMLEntry intentionally omits the `id` field that the seed
// loader stamps after parsing — fresh entries don't need it; the
// FileStore assigns runtime IDs at boot.
type orgTOMLEntry struct {
	Slug        string   `toml:"slug"`
	Name        string   `toml:"name"`
	ShortDesc   string   `toml:"short_desc"`
	WebsiteURL  string   `toml:"website_url"`
	ContactURL  string   `toml:"contact_url,omitempty"`
	Tags        []string `toml:"tags"`
	RegionSlugs []string `toml:"region_slugs"`
}

type orgTOMLDoc struct {
	Org []orgTOMLEntry `toml:"org"`
}
