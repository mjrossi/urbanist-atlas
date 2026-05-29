package seedfiles

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// SubmitterFieldLimits are the caps the handler enforces on the
// optional submitter_* metadata fields (name / email / note). They
// live here so both the loader and the public submissions handler
// agree on the shape of an acceptable payload.
//
// submitter_email is bounded by RFC 5321's 320-character envelope
// (local 64 + "@" + domain 255). submitter_note is the largest by
// far because moderators want context, but we still cap it so a
// single submission can't pin a worker on JSON parsing or stuff a
// multi-MB blob into orgs.toml prep notes.
const (
	MaxSubmitterNameLen  = 200
	MaxSubmitterEmailLen = 320
	MaxSubmitterNoteLen  = 2000
)

// SubmissionPayloadInput is the subset of fields the public
// submissions endpoint accepts, in shape-only form (no atlas / oapi
// dependency). The HTTP handler builds it from the JSON body and
// passes it through ValidateSubmissionPayload.
type SubmissionPayloadInput struct {
	Name        string
	ShortDesc   string
	WebsiteURL  string
	ContactURL  string
	Tags        []string
	RegionSlugs []string
}

// SubmitterInput is the optional submitter metadata attached to a
// public submission. Empty strings are treated as absent.
type SubmitterInput struct {
	Name  string
	Email string
	Note  string
}

// ValidateSubmissionPayload runs the per-field checks shared between
// the seed-file loader (via ValidateOrgFields) and the public
// submissions handler. It returns a map keyed by JSON field name with
// a one-sentence message per offending field, or nil when the payload
// passes.
//
// Field keys match the wire shape (`name`, `short_desc`,
// `website_url`, `contact_url`, `tags`, `region_slugs`,
// `submitter_name`, `submitter_email`, `submitter_note`) so the
// frontend can plug them straight into react-hook-form's setError.
// The values are sentences (end with a period) safe to render to end
// users.
//
// Region-slug *existence* (do these slugs match a real region in the
// store?) is NOT checked here — that requires a context-bound
// atlas.Store lookup and lives in the handler. This validator covers
// the static shape rules only.
func ValidateSubmissionPayload(p SubmissionPayloadInput, s SubmitterInput) map[string]string {
	errs := map[string]string{}

	if p.Name == "" {
		errs["name"] = "Name is required."
	} else if utf8.RuneCountInString(p.Name) > MaxNameLen {
		errs["name"] = fmt.Sprintf("Name must be at most %d characters.", MaxNameLen)
	}

	if p.ShortDesc == "" {
		errs["short_desc"] = "A short description is required."
	} else if utf8.RuneCountInString(p.ShortDesc) > MaxShortDescLen {
		errs["short_desc"] = fmt.Sprintf("Short description must be at most %d characters.", MaxShortDescLen)
	}

	if p.WebsiteURL == "" {
		errs["website_url"] = "A website URL is required."
	} else if msg := checkHTTPURL(p.WebsiteURL); msg != "" {
		errs["website_url"] = msg
	}

	if p.ContactURL != "" {
		if msg := checkContactURL(p.ContactURL); msg != "" {
			errs["contact_url"] = msg
		}
	}

	if len(p.Tags) > MaxTags {
		errs["tags"] = fmt.Sprintf("Tags must have at most %d entries.", MaxTags)
	} else {
		for i, tag := range p.Tags {
			if tag == "" {
				errs["tags"] = fmt.Sprintf("Tag at index %d is empty.", i)
				break
			}
			if utf8.RuneCountInString(tag) > MaxTagLen {
				errs["tags"] = fmt.Sprintf("Tag at index %d must be at most %d characters.", i, MaxTagLen)
				break
			}
		}
	}

	// region_slugs is optional on the public-submission wire: the SPA's
	// region field is free-form text (most submitters don't know the
	// canonical slug — e.g. nyc-tri-state, washington-dc-msa), so we
	// trust editors to finalize the slug in PR review. Loader-side
	// validation (ValidateOrgFields) still requires at least one entry
	// for orgs.toml records that are already in the dataset.
	if len(p.RegionSlugs) > MaxRegionSlugs {
		errs["region_slugs"] = fmt.Sprintf("Region slugs must have at most %d entries.", MaxRegionSlugs)
	} else {
		for i, slug := range p.RegionSlugs {
			if slug == "" {
				errs["region_slugs"] = fmt.Sprintf("Region slug at index %d is empty.", i)
				break
			}
			if len(slug) > MaxRegionSlugLen {
				errs["region_slugs"] = fmt.Sprintf("Region slug at index %d must be at most %d characters.", i, MaxRegionSlugLen)
				break
			}
		}
	}

	if utf8.RuneCountInString(s.Name) > MaxSubmitterNameLen {
		errs["submitter_name"] = fmt.Sprintf("Submitter name must be at most %d characters.", MaxSubmitterNameLen)
	}
	if len(s.Email) > MaxSubmitterEmailLen {
		errs["submitter_email"] = fmt.Sprintf("Submitter email must be at most %d characters.", MaxSubmitterEmailLen)
	} else if s.Email != "" && !looksLikeEmail(s.Email) {
		// Cheap shape check: exactly one '@', non-empty local and
		// domain, and a '.' somewhere in the domain. Catches the worst
		// junk without pretending to do full RFC 5322 — strict
		// validation would reject perfectly valid addresses and we
		// don't need that strictness for a contact field.
		errs["submitter_email"] = "Submitter email is not a valid email address."
	}
	if utf8.RuneCountInString(s.Note) > MaxSubmitterNoteLen {
		errs["submitter_note"] = fmt.Sprintf("Submitter note must be at most %d characters.", MaxSubmitterNoteLen)
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

func looksLikeEmail(addr string) bool {
	at := strings.IndexByte(addr, '@')
	if at <= 0 || at != strings.LastIndexByte(addr, '@') || at == len(addr)-1 {
		return false
	}
	return strings.ContainsRune(addr[at+1:], '.')
}

// checkHTTPURL mirrors validateHTTPURL but returns the message as a
// caller-renderable sentence (or "" on success). Kept separate to
// avoid disturbing the existing error wrapping in the loader path.
func checkHTTPURL(raw string) string {
	if len(raw) > MaxURLLen {
		return fmt.Sprintf("Website URL must be at most %d characters.", MaxURLLen)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "Website URL is not a valid URL."
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "Website URL must use http or https."
	}
	if u.Host == "" {
		return "Website URL must include a host."
	}
	return ""
}

// checkContactURL mirrors validateContactURL but returns a
// caller-renderable sentence. mailto: is accepted alongside http(s).
func checkContactURL(raw string) string {
	if len(raw) > MaxURLLen {
		return fmt.Sprintf("Contact URL must be at most %d characters.", MaxURLLen)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "Contact URL is not a valid URL."
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "Contact URL must include a host."
		}
		return ""
	case "mailto":
		if u.Opaque == "" {
			return "Contact URL mailto: must include an address."
		}
		return ""
	default:
		return "Contact URL must use http, https, or mailto."
	}
}
