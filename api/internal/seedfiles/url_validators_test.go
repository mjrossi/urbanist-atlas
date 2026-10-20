package seedfiles

import (
	"strings"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// TestURLValidators_AgreeWithClassifyURL pins that the loader's
// error-returning URL validators reject exactly what atlas.ClassifyURL
// rejects. Public submissions format the same classification as
// sentences (pinned in pkg/atlas), so the two paths cannot drift.
func TestURLValidators_AgreeWithClassifyURL(t *testing.T) {
	httpCases := []string{
		"http://example.org",
		"https://example.org/path",
		"javascript:alert(1)",
		"ftp://example.org/file",
		"mailto:hi@example.org",
		"example.org",
		"https://",
		"%zz",
		"https://x.org/" + strings.Repeat("a", atlas.MaxURLLen),
	}
	for _, raw := range httpCases {
		errRejected := validateHTTPURL("website_url", raw) != nil
		reason, _ := atlas.ClassifyURL(raw, false)
		if classRejected := reason != atlas.URLOK; errRejected != classRejected {
			t.Errorf("http URL %q: loader rejected=%v, ClassifyURL rejected=%v (must agree)",
				raw, errRejected, classRejected)
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
		"https://x.org/" + strings.Repeat("a", atlas.MaxURLLen),
	}
	for _, raw := range contactCases {
		errRejected := validateContactURL(raw) != nil
		reason, _ := atlas.ClassifyURL(raw, true)
		if classRejected := reason != atlas.URLOK; errRejected != classRejected {
			t.Errorf("contact URL %q: loader rejected=%v, ClassifyURL rejected=%v (must agree)",
				raw, errRejected, classRejected)
		}
	}
}
