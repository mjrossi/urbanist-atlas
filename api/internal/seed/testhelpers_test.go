package seed

import (
	"os"
	"path/filepath"
	"testing"
)

// openTestFile walks up from the test cwd to find api/seed/orgs.yaml.
// Returns the file (caller closes) or an error if not reachable.
func openTestFile(t *testing.T) (*os.File, error) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	// Walk up to repo root; the file lives at api/seed/orgs.yaml.
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "seed", "orgs.yaml")
		if f, err := os.Open(candidate); err == nil {
			return f, nil
		}
		dir = filepath.Dir(dir)
	}
	return os.Open(filepath.Join("seed", "orgs.yaml"))
}
