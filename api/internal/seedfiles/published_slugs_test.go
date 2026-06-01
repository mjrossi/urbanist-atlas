package seedfiles_test

import (
	"flag"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
	seedfs "github.com/mjrossi/urbanist-atlas/api/seed"
)

// updateSlugs regenerates testdata/published_slugs.golden from the
// current embedded seed. This is the deliberate same-PR escape hatch: a
// maintainer who legitimately retires or renames a published slug runs
//
//	go test ./internal/seedfiles/ -run TestPublishedSlugs -update
//
// in the SAME PR as the slug change, so the diff shows exactly which
// public slug moved — a reviewable act, not a silent break. Mirrors the
// stage-regenerate-commit flow used to resolve seed-check drift.
var updateSlugs = flag.Bool("update", false, "regenerate published_slugs.golden from the current seed")

const publishedSlugsGolden = "testdata/published_slugs.golden"

// TestPublishedSlugs is the D-01 append-only slug guard. Slugs are
// permanent public identifiers (the /api/v1/regions/{slug} contract,
// docs/region-graph.md "Slug permanence — append, never rename"); once
// published, a slug may be added to but never removed or renamed.
//
// The committed golden is a sorted snapshot of every published region
// slug — states, hand-curated leaves, AND generated MSA/CMA slugs (the
// FULL set via MemStore.Slugs(), not the browseable-only ListRegions
// set). The assertion direction is golden ⊆ current (RESEARCH
// Mechanism #1, Pitfall 3): every previously-published slug must still
// exist; NEW slugs are allowed (append-only). A removed/renamed slug
// fails, naming the missing slug and the -update escape hatch.
func TestPublishedSlugs(t *testing.T) {
	store, err := seedfiles.BuildMemStore(nil, seedfs.FS)
	if err != nil {
		t.Fatalf("BuildMemStore embed: %v", err)
	}
	current := store.Slugs()
	slices.Sort(current)

	if *updateSlugs {
		body := strings.Join(current, "\n") + "\n"
		if err := os.WriteFile(publishedSlugsGolden, []byte(body), 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
		t.Logf("wrote %d slugs to %s", len(current), publishedSlugsGolden)
		return
	}

	data, err := os.ReadFile(publishedSlugsGolden)
	if err != nil {
		t.Fatalf("read golden %s (run with -update first): %v", publishedSlugsGolden, err)
	}
	golden := nonEmptyLines(string(data))
	if len(golden) == 0 {
		t.Fatalf("%s is empty — run with -update to seed it", publishedSlugsGolden)
	}

	currentSet := make(map[string]bool, len(current))
	for _, s := range current {
		currentSet[s] = true
	}
	for _, want := range golden {
		if !currentSet[want] {
			t.Errorf("published slug %q disappeared from the seed (rename/removal). "+
				"Slugs are permanent public identifiers — append, never rename "+
				"(docs/region-graph.md). If this removal is deliberate, run "+
				"`go test ./internal/seedfiles/ -run TestPublishedSlugs -update` in the "+
				"SAME PR and review the diff.", want)
		}
	}
}

// TestPublishedSlugs_AppendOnlyDirection pins the assertion semantics
// directly (independent of the live golden file): golden ⊆ current
// means a simulated REMOVAL fails and a simulated ADDITION passes.
func TestPublishedSlugs_AppendOnlyDirection(t *testing.T) {
	store, err := seedfiles.BuildMemStore(nil, seedfs.FS)
	if err != nil {
		t.Fatalf("BuildMemStore embed: %v", err)
	}
	current := store.Slugs()
	if len(current) == 0 {
		t.Fatal("seed produced zero slugs")
	}
	currentSet := make(map[string]bool, len(current))
	for _, s := range current {
		currentSet[s] = true
	}

	// Simulated REMOVAL: a golden carrying a slug not in current must
	// be detected as missing (the guard would fail).
	removed := append(slices.Clone(current), "definitely-not-a-real-slug-xyz")
	missing := missingFromCurrent(removed, currentSet)
	if len(missing) != 1 || missing[0] != "definitely-not-a-real-slug-xyz" {
		t.Errorf("removal not detected: missing=%v", missing)
	}

	// Simulated ADDITION: current having an extra slug beyond the
	// golden must NOT be flagged (append-only). Use the real current
	// set as the golden and a superset as current.
	goldenSubset := current[:len(current)-1] // drop one → golden ⊂ current
	if extra := missingFromCurrent(goldenSubset, currentSet); len(extra) != 0 {
		t.Errorf("addition wrongly flagged: %v", extra)
	}
}

// nonEmptyLines splits s on newlines, trimming and dropping blanks.
func nonEmptyLines(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

// missingFromCurrent returns the golden slugs absent from currentSet —
// the same golden ⊆ current check the guard performs.
func missingFromCurrent(golden []string, currentSet map[string]bool) []string {
	var missing []string
	for _, g := range golden {
		if !currentSet[g] {
			missing = append(missing, g)
		}
	}
	return missing
}
