package atlas

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

// Field caps shared by the seed loader and public submissions. They are
// generous next to the seed data but stop one submission from storing
// megabytes or rendering an unreadable orgs.toml entry.
const (
	MaxNameLen       = 200
	MaxShortDescLen  = 1000
	MaxURLLen        = 500
	MaxTagLen        = 50
	MaxTags          = 20
	MaxRegionSlugLen = 100
	MaxRegionSlugs   = 20

	MaxSubmitterNameLen  = 200
	MaxSubmitterEmailLen = 320 // RFC 5321 envelope: local 64 + "@" + domain 255
	MaxSubmitterNoteLen  = 2000
)

// SubmissionValidationError reports the submission fields that failed
// validation. Fields maps a wire field name to a one-sentence message
// safe to show the submitter.
type SubmissionValidationError struct {
	Fields map[string]string
}

func (e *SubmissionValidationError) Error() string {
	names := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		names = append(names, k)
	}
	slices.Sort(names)
	return "atlas: invalid submission: " + strings.Join(names, ", ")
}

// CreateSubmission validates in with ValidateSubmission and, if it
// passes, stores the normalized input in subs. A validation failure is a
// *SubmissionValidationError; any other error comes from the store.
func CreateSubmission(ctx context.Context, regions Store, subs SubmissionStore, in NewSubmissionInput) (Submission, error) {
	in, err := ValidateSubmission(ctx, regions, in)
	if err != nil {
		return Submission{}, err
	}
	sub, err := subs.Create(ctx, in)
	if err != nil {
		return Submission{}, fmt.Errorf("atlas: create submission: %w", err)
	}
	return sub, nil
}

// ValidateSubmission trims in, checks every field's shape, and checks
// each region slug resolves in regions. It returns the normalized input,
// or a *SubmissionValidationError naming every offending field.
// SubmitterNote is kept verbatim, and blank region slugs are dropped.
func ValidateSubmission(ctx context.Context, regions Store, in NewSubmissionInput) (NewSubmissionInput, error) {
	in = normalizeSubmission(in)
	errs := submissionShapeErrors(in)
	// Existence is checked only once the slugs are well-formed, so the
	// client gets one message per field.
	if _, bad := errs["region_slugs"]; !bad {
		if msg := checkRegionSlugsExist(ctx, regions, in.Payload.RegionSlugs); msg != "" {
			errs["region_slugs"] = msg
		}
	}
	if len(errs) > 0 {
		return NewSubmissionInput{}, &SubmissionValidationError{Fields: errs}
	}
	return in, nil
}

func normalizeSubmission(in NewSubmissionInput) NewSubmissionInput {
	slugs := make([]string, 0, len(in.Payload.RegionSlugs))
	for _, s := range in.Payload.RegionSlugs {
		if t := strings.TrimSpace(s); t != "" {
			slugs = append(slugs, t)
		}
	}
	return NewSubmissionInput{
		Payload: SubmissionPayload{
			Name:        strings.TrimSpace(in.Payload.Name),
			ShortDesc:   strings.TrimSpace(in.Payload.ShortDesc),
			WebsiteURL:  strings.TrimSpace(in.Payload.WebsiteURL),
			ContactURL:  strings.TrimSpace(in.Payload.ContactURL),
			Tags:        in.Payload.Tags,
			RegionSlugs: slugs,
		},
		SubmitterName:  strings.TrimSpace(in.SubmitterName),
		SubmitterEmail: strings.TrimSpace(in.SubmitterEmail),
		SubmitterNote:  in.SubmitterNote,
	}
}

func submissionShapeErrors(in NewSubmissionInput) map[string]string {
	p := in.Payload
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

	// Unlike the seed loader, zero region slugs is fine: submitters
	// rarely know the canonical slug, so editors set it in PR review.
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

	if utf8.RuneCountInString(in.SubmitterName) > MaxSubmitterNameLen {
		errs["submitter_name"] = fmt.Sprintf("Submitter name must be at most %d characters.", MaxSubmitterNameLen)
	}
	if len(in.SubmitterEmail) > MaxSubmitterEmailLen {
		errs["submitter_email"] = fmt.Sprintf("Submitter email must be at most %d characters.", MaxSubmitterEmailLen)
	} else if in.SubmitterEmail != "" && !looksLikeEmail(in.SubmitterEmail) {
		errs["submitter_email"] = "Submitter email is not a valid email address."
	}
	if utf8.RuneCountInString(in.SubmitterNote) > MaxSubmitterNoteLen {
		errs["submitter_note"] = fmt.Sprintf("Submitter note must be at most %d characters.", MaxSubmitterNoteLen)
	}

	return errs
}

// looksLikeEmail is a deliberately loose shape check: one '@', a
// non-empty local part, and a '.' in the domain. Full RFC 5322 parsing
// would reject valid addresses, and this is only a contact field.
func looksLikeEmail(addr string) bool {
	at := strings.IndexByte(addr, '@')
	if at <= 0 || at != strings.LastIndexByte(addr, '@') || at == len(addr)-1 {
		return false
	}
	return strings.ContainsRune(addr[at+1:], '.')
}

// checkRegionSlugsExist returns a message naming the first duplicate or
// unresolvable slug, or "" when every slug resolves.
func checkRegionSlugsExist(ctx context.Context, regions Store, slugs []string) string {
	if regions == nil {
		return "Region resolver is unavailable; cannot verify region slugs."
	}
	seen := map[string]bool{}
	for _, slug := range slugs {
		if seen[slug] {
			return fmt.Sprintf("Region slug %q appears more than once.", slug)
		}
		seen[slug] = true
		if _, err := regions.ResolveRegionBySlug(ctx, slug); err != nil {
			if errors.Is(err, ErrRegionNotFound) {
				return fmt.Sprintf("Region slug %q does not match any known region.", slug)
			}
			return fmt.Sprintf("Region lookup failed for %q.", slug)
		}
	}
	return ""
}

// URLReason is the outcome of ClassifyURL.
type URLReason int

// ClassifyURL outcomes. URLMailtoNoAddress is only returned when mailto
// is allowed; otherwise a mailto URL is URLBadScheme.
const (
	URLOK URLReason = iota
	URLTooLong
	URLParseFailed
	URLBadScheme
	URLMissingHost
	URLMailtoNoAddress
)

// ClassifyURL is the one URL check behind both the seed loader and
// public submissions, so the two accept exactly the same org URLs. It
// requires an absolute http(s) URL with a host, or, when allowMailto is
// set, a mailto with an address. The error is the url.Parse error, set
// only for URLParseFailed.
func ClassifyURL(raw string, allowMailto bool) (URLReason, error) {
	if len(raw) > MaxURLLen {
		return URLTooLong, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return URLParseFailed, err
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return URLMissingHost, nil
		}
		return URLOK, nil
	case "mailto":
		if !allowMailto {
			return URLBadScheme, nil
		}
		if u.Opaque == "" {
			return URLMailtoNoAddress, nil
		}
		return URLOK, nil
	default:
		return URLBadScheme, nil
	}
}

func checkHTTPURL(raw string) string {
	reason, _ := ClassifyURL(raw, false)
	switch reason {
	case URLOK:
		return ""
	case URLTooLong:
		return fmt.Sprintf("Website URL must be at most %d characters.", MaxURLLen)
	case URLParseFailed:
		return "Website URL is not a valid URL."
	case URLMissingHost:
		return "Website URL must include a host."
	default:
		return "Website URL must use http or https."
	}
}

func checkContactURL(raw string) string {
	reason, _ := ClassifyURL(raw, true)
	switch reason {
	case URLOK:
		return ""
	case URLTooLong:
		return fmt.Sprintf("Contact URL must be at most %d characters.", MaxURLLen)
	case URLParseFailed:
		return "Contact URL is not a valid URL."
	case URLMissingHost:
		return "Contact URL must include a host."
	case URLMailtoNoAddress:
		return "Contact URL mailto: must include an address."
	default:
		return "Contact URL must use http, https, or mailto."
	}
}
