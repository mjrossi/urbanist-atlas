package atlas

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func submissionRegions(t *testing.T) *MemStore {
	t.Helper()
	s := NewMemStore()
	LoadDevFixtures(s)
	return s
}

func goodSubmissionInput() NewSubmissionInput {
	return NewSubmissionInput{
		Payload: SubmissionPayload{
			Name:        "Brooklyn Greenways",
			ShortDesc:   "Volunteers expanding the protected-lane network.",
			WebsiteURL:  "https://example.org/brooklyn-greenways",
			RegionSlugs: []string{"brooklyn-ny"},
		},
		SubmitterName:  "Jane",
		SubmitterEmail: "jane@example.org",
	}
}

func fieldErrorsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	var verr *SubmissionValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want *SubmissionValidationError", err)
	}
	return verr.Fields
}

func TestValidateSubmission_TrimsAndDropsBlankSlugs(t *testing.T) {
	in := NewSubmissionInput{
		Payload: SubmissionPayload{
			Name:        "  Brooklyn Greenways ",
			ShortDesc:   "\tVolunteers. ",
			WebsiteURL:  " https://example.org ",
			ContactURL:  " mailto:hi@example.org ",
			RegionSlugs: []string{" brooklyn-ny ", "  ", ""},
		},
		SubmitterName:  " Jane ",
		SubmitterEmail: " jane@example.org ",
		SubmitterNote:  "  keep my spacing  ",
	}
	got, err := ValidateSubmission(context.Background(), submissionRegions(t), in)
	if err != nil {
		t.Fatalf("ValidateSubmission: %v", err)
	}
	want := NewSubmissionInput{
		Payload: SubmissionPayload{
			Name:        "Brooklyn Greenways",
			ShortDesc:   "Volunteers.",
			WebsiteURL:  "https://example.org",
			ContactURL:  "mailto:hi@example.org",
			RegionSlugs: []string{"brooklyn-ny"},
		},
		SubmitterName:  "Jane",
		SubmitterEmail: "jane@example.org",
		SubmitterNote:  "  keep my spacing  ",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("normalized input (-want +got):\n%s", diff)
	}
}

// The stored slugs feed the generated orgs.toml block, whose loader
// matches slugs exactly, so a differently-cased slug must be stored in
// its canonical form.
func TestValidateSubmission_CanonicalizesRegionSlugCase(t *testing.T) {
	in := goodSubmissionInput()
	in.Payload.RegionSlugs = []string{"Brooklyn-NY", "NY"}
	got, err := ValidateSubmission(context.Background(), submissionRegions(t), in)
	if err != nil {
		t.Fatalf("ValidateSubmission: %v", err)
	}
	if diff := cmp.Diff([]string{"brooklyn-ny", "ny"}, got.Payload.RegionSlugs); diff != "" {
		t.Errorf("RegionSlugs (-want +got):\n%s", diff)
	}
}

func TestValidateSubmission_NoRegionSlugsIsValid(t *testing.T) {
	in := goodSubmissionInput()
	in.Payload.RegionSlugs = nil
	got, err := ValidateSubmission(context.Background(), submissionRegions(t), in)
	if err != nil {
		t.Fatalf("ValidateSubmission: %v", err)
	}
	if got.Payload.RegionSlugs == nil || len(got.Payload.RegionSlugs) != 0 {
		t.Errorf("RegionSlugs = %#v, want empty non-nil slice", got.Payload.RegionSlugs)
	}
}

func TestValidateSubmission_FieldErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NewSubmissionInput)
		want   map[string]string
	}{
		{
			name:   "missing name",
			mutate: func(in *NewSubmissionInput) { in.Payload.Name = "   " },
			want:   map[string]string{"name": "Name is required."},
		},
		{
			name:   "name too long",
			mutate: func(in *NewSubmissionInput) { in.Payload.Name = strings.Repeat("a", MaxNameLen+1) },
			want:   map[string]string{"name": "Name must be at most 200 characters."},
		},
		{
			name:   "missing short_desc",
			mutate: func(in *NewSubmissionInput) { in.Payload.ShortDesc = "" },
			want:   map[string]string{"short_desc": "A short description is required."},
		},
		{
			name:   "missing website",
			mutate: func(in *NewSubmissionInput) { in.Payload.WebsiteURL = "" },
			want:   map[string]string{"website_url": "A website URL is required."},
		},
		{
			name:   "non-http website",
			mutate: func(in *NewSubmissionInput) { in.Payload.WebsiteURL = "javascript:alert(1)" },
			want:   map[string]string{"website_url": "Website URL must use http or https."},
		},
		{
			name:   "bad contact",
			mutate: func(in *NewSubmissionInput) { in.Payload.ContactURL = "ftp://example.org" },
			want:   map[string]string{"contact_url": "Contact URL must use http, https, or mailto."},
		},
		{
			name: "too many slugs skips existence check",
			mutate: func(in *NewSubmissionInput) {
				in.Payload.RegionSlugs = make([]string, MaxRegionSlugs+1)
				for i := range in.Payload.RegionSlugs {
					in.Payload.RegionSlugs[i] = "no-such-region"
				}
			},
			want: map[string]string{"region_slugs": "Region slugs must have at most 20 entries."},
		},
		{
			name: "slug too long",
			mutate: func(in *NewSubmissionInput) {
				in.Payload.RegionSlugs = []string{strings.Repeat("a", MaxRegionSlugLen+1)}
			},
			want: map[string]string{"region_slugs": "Region slug at index 0 must be at most 100 characters."},
		},
		{
			name:   "unknown slug",
			mutate: func(in *NewSubmissionInput) { in.Payload.RegionSlugs = []string{"brooklyn-ny", "no-such-region"} },
			want:   map[string]string{"region_slugs": `Region slug "no-such-region" does not match any known region.`},
		},
		{
			name:   "duplicate slug",
			mutate: func(in *NewSubmissionInput) { in.Payload.RegionSlugs = []string{"brooklyn-ny", " brooklyn-ny"} },
			want:   map[string]string{"region_slugs": `Region slug "brooklyn-ny" appears more than once.`},
		},
		{
			name:   "duplicate slug differing only in case",
			mutate: func(in *NewSubmissionInput) { in.Payload.RegionSlugs = []string{"brooklyn-ny", "Brooklyn-NY"} },
			want:   map[string]string{"region_slugs": `Region slug "Brooklyn-NY" appears more than once.`},
		},
		{
			name:   "submitter name too long",
			mutate: func(in *NewSubmissionInput) { in.SubmitterName = strings.Repeat("a", MaxSubmitterNameLen+1) },
			want:   map[string]string{"submitter_name": "Submitter name must be at most 200 characters."},
		},
		{
			name:   "malformed email",
			mutate: func(in *NewSubmissionInput) { in.SubmitterEmail = "not-an-email" },
			want:   map[string]string{"submitter_email": "Submitter email is not a valid email address."},
		},
		{
			name:   "email too long",
			mutate: func(in *NewSubmissionInput) { in.SubmitterEmail = strings.Repeat("a", MaxSubmitterEmailLen) + "@x.org" },
			want:   map[string]string{"submitter_email": "Submitter email must be at most 320 characters."},
		},
		{
			name:   "note too long",
			mutate: func(in *NewSubmissionInput) { in.SubmitterNote = strings.Repeat("a", MaxSubmitterNoteLen+1) },
			want:   map[string]string{"submitter_note": "Submitter note must be at most 2000 characters."},
		},
		{
			name: "several fields at once",
			mutate: func(in *NewSubmissionInput) {
				in.Payload.Name = ""
				in.SubmitterEmail = "nope"
			},
			want: map[string]string{
				"name":            "Name is required.",
				"submitter_email": "Submitter email is not a valid email address.",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := goodSubmissionInput()
			tc.mutate(&in)
			_, err := ValidateSubmission(context.Background(), submissionRegions(t), in)
			if diff := cmp.Diff(tc.want, fieldErrorsOf(t, err)); diff != "" {
				t.Errorf("field errors (-want +got):\n%s", diff)
			}
		})
	}
}

type failingSlugStore struct {
	Store
}

func (failingSlugStore) ResolveRegionBySlug(context.Context, string) (Region, error) {
	return Region{}, errors.New("boom")
}

// A store failure while resolving a slug is reported on the field, not
// as an internal error, so the client still gets a per-field response.
func TestValidateSubmission_RegionLookupFailure(t *testing.T) {
	_, err := ValidateSubmission(context.Background(), failingSlugStore{}, goodSubmissionInput())
	want := map[string]string{"region_slugs": `Region lookup failed for "brooklyn-ny".`}
	if diff := cmp.Diff(want, fieldErrorsOf(t, err)); diff != "" {
		t.Errorf("field errors (-want +got):\n%s", diff)
	}
}

func TestValidateSubmission_NilRegions(t *testing.T) {
	_, err := ValidateSubmission(context.Background(), nil, goodSubmissionInput())
	want := map[string]string{"region_slugs": "Region resolver is unavailable; cannot verify region slugs."}
	if diff := cmp.Diff(want, fieldErrorsOf(t, err)); diff != "" {
		t.Errorf("field errors (-want +got):\n%s", diff)
	}
}

func TestSubmissionValidationError_ErrorNamesFieldsSorted(t *testing.T) {
	err := &SubmissionValidationError{Fields: map[string]string{"website_url": "x", "name": "y"}}
	if got, want := err.Error(), "atlas: invalid submission: name, website_url"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

type recordingSubmissionStore struct {
	SubmissionStore
	created []NewSubmissionInput
	err     error
}

func (s *recordingSubmissionStore) Create(_ context.Context, in NewSubmissionInput) (Submission, error) {
	if s.err != nil {
		return Submission{}, s.err
	}
	s.created = append(s.created, in)
	return Submission{PublicID: "id", Payload: in.Payload, Status: SubmissionPending}, nil
}

func TestCreateSubmission_StoresNormalizedInput(t *testing.T) {
	subs := &recordingSubmissionStore{}
	in := goodSubmissionInput()
	in.Payload.Name = " Brooklyn Greenways "
	sub, err := CreateSubmission(context.Background(), submissionRegions(t), subs, in)
	if err != nil {
		t.Fatalf("CreateSubmission: %v", err)
	}
	if len(subs.created) != 1 || subs.created[0].Payload.Name != "Brooklyn Greenways" {
		t.Fatalf("created = %#v, want one trimmed submission", subs.created)
	}
	if sub.Status != SubmissionPending {
		t.Errorf("status = %q, want pending", sub.Status)
	}
}

func TestCreateSubmission_InvalidInputIsNotStored(t *testing.T) {
	subs := &recordingSubmissionStore{}
	in := goodSubmissionInput()
	in.Payload.Name = ""
	_, err := CreateSubmission(context.Background(), submissionRegions(t), subs, in)
	fieldErrorsOf(t, err)
	if len(subs.created) != 0 {
		t.Errorf("created = %d rows, want 0", len(subs.created))
	}
}

func TestCreateSubmission_StoreErrorIsWrapped(t *testing.T) {
	boom := errors.New("disk full")
	_, err := CreateSubmission(context.Background(), submissionRegions(t), &recordingSubmissionStore{err: boom}, goodSubmissionInput())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapping %v", err, boom)
	}
	var verr *SubmissionValidationError
	if errors.As(err, &verr) {
		t.Fatalf("store error reported as validation error: %v", err)
	}
}

func TestCheckHTTPURL_Sentences(t *testing.T) {
	tests := []struct {
		name, raw, want string
	}{
		{"valid http", "http://example.org", ""},
		{"valid https", "https://example.org/path", ""},
		{"javascript scheme", "javascript:alert(1)", "Website URL must use http or https."},
		{"mailto rejected", "mailto:hi@example.org", "Website URL must use http or https."},
		{"no scheme", "example.org", "Website URL must use http or https."},
		{"missing host", "https://", "Website URL must include a host."},
		{"parse failure", "%zz", "Website URL is not a valid URL."},
		{"too long", "https://x.org/" + strings.Repeat("a", MaxURLLen), "Website URL must be at most 500 characters."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkHTTPURL(tc.raw); got != tc.want {
				t.Fatalf("checkHTTPURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			if rejected, classified := tc.want != "", classifyOnly(tc.raw, false) != URLOK; rejected != classified {
				t.Fatalf("checkHTTPURL and ClassifyURL disagree on %q", tc.raw)
			}
		})
	}
}

func TestCheckContactURL_Sentences(t *testing.T) {
	tests := []struct {
		name, raw, want string
	}{
		{"valid https", "https://example.org/contact", ""},
		{"valid mailto", "mailto:hi@example.org", ""},
		{"bad scheme", "ftp://example.org", "Contact URL must use http, https, or mailto."},
		{"missing host", "http://", "Contact URL must include a host."},
		{"empty mailto", "mailto:", "Contact URL mailto: must include an address."},
		{"parse failure", "%zz", "Contact URL is not a valid URL."},
		{"too long", "https://x.org/" + strings.Repeat("a", MaxURLLen), "Contact URL must be at most 500 characters."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkContactURL(tc.raw); got != tc.want {
				t.Fatalf("checkContactURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			if rejected, classified := tc.want != "", classifyOnly(tc.raw, true) != URLOK; rejected != classified {
				t.Fatalf("checkContactURL and ClassifyURL disagree on %q", tc.raw)
			}
		})
	}
}

func classifyOnly(raw string, allowMailto bool) URLReason {
	reason, _ := ClassifyURL(raw, allowMailto)
	return reason
}
