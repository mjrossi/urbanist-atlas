package githubpr

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// RenderOrgBlock returns the [[org]] block to append to orgs.toml for an
// approved submission. Output is deterministic, since the seed-file diff
// is what editors review. addedAt is the approval date, not the
// submission date: added_at means when the org joined the atlas.
func RenderOrgBlock(sub atlas.Submission, slug string, addedAt time.Time) (string, error) {
	if slug == "" {
		return "", fmt.Errorf("githubpr: empty slug for submission %s", sub.PublicID)
	}
	// Submissions carry no tags; render `tags = []` for the editor to fill
	// in.
	tags := sub.Payload.Tags
	if tags == nil {
		tags = []string{}
	}
	entry := orgTOMLEntry{
		Slug:        slug,
		AddedAt:     toml.LocalDate{Year: addedAt.Year(), Month: int(addedAt.Month()), Day: addedAt.Day()},
		Name:        sub.Payload.Name,
		ShortDesc:   sub.Payload.ShortDesc,
		WebsiteURL:  sub.Payload.WebsiteURL,
		ContactURL:  sub.Payload.ContactURL,
		Tags:        tags,
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

// DeriveSlug returns a starting-point slug: lowercase ASCII letters and
// digits, hyphen-joined. Other runes drop out ("Café Réforme" →
// "caf-r-forme"); editors fix such slugs in PR review rather than the
// binary carrying a transliteration dependency.
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

// orgTOMLEntry has no id: the loader assigns IDs at boot.
type orgTOMLEntry struct {
	Slug        string         `toml:"slug"`
	AddedAt     toml.LocalDate `toml:"added_at"`
	Name        string         `toml:"name"`
	ShortDesc   string         `toml:"short_desc"`
	WebsiteURL  string         `toml:"website_url"`
	ContactURL  string         `toml:"contact_url,omitempty"`
	Tags        []string       `toml:"tags"`
	RegionSlugs []string       `toml:"region_slugs"`
}

type orgTOMLDoc struct {
	Org []orgTOMLEntry `toml:"org"`
}
