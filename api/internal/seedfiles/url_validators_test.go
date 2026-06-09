package seedfiles

import (
	"strings"
	"testing"
)

// The website_url and contact_url validators come in two caller styles
// that share one core (classifyURL): an error-returning pair used by the
// seed loader (validateHTTPURL / validateContactURL) and a
// sentence-returning pair used by the public submissions handler
// (checkHTTPURL / checkContactURL). These tests pin the sentence wording
// — which had no coverage before the consolidation — and assert that
// both styles agree on accept/reject for every input, so the shared
// security-relevant parse boundary can't drift between the two paths.

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
		})
	}
}

// TestURLValidators_ErrorAndSentenceAgree guards the consolidation: for
// the same input, the error-returning loader validator and the
// sentence-returning handler validator must reach the same accept/reject
// verdict. If a future edit to classifyURL (or either formatter) made one
// style accept what the other rejects, this fails.
func TestURLValidators_ErrorAndSentenceAgree(t *testing.T) {
	httpCases := []string{
		"http://example.org",
		"https://example.org/path",
		"javascript:alert(1)",
		"ftp://example.org/file",
		"mailto:hi@example.org",
		"example.org",
		"https://",
		"%zz",
		"https://x.org/" + strings.Repeat("a", MaxURLLen),
	}
	for _, raw := range httpCases {
		errRejected := validateHTTPURL("website_url", raw) != nil
		sentRejected := checkHTTPURL(raw) != ""
		if errRejected != sentRejected {
			t.Errorf("http URL %q: error-style rejected=%v, sentence-style rejected=%v (must agree)",
				raw, errRejected, sentRejected)
		}
	}

	contactCases := []string{
		"https://example.org/contact",
		"mailto:hi@example.org",
		"mailto:",
		"ftp://example.org",
		"http://",
		"%zz",
		"javascript:alert(1)",
		"https://x.org/" + strings.Repeat("a", MaxURLLen),
	}
	for _, raw := range contactCases {
		errRejected := validateContactURL(raw) != nil
		sentRejected := checkContactURL(raw) != ""
		if errRejected != sentRejected {
			t.Errorf("contact URL %q: error-style rejected=%v, sentence-style rejected=%v (must agree)",
				raw, errRejected, sentRejected)
		}
	}
}
