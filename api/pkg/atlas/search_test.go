package atlas

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// newSearchFixture builds a small graph that exercises ranking, the
// national-tier exclusion, and the state-ancestor context label —
// including two same-named "Springfield" cities in different states so
// the disambiguation hint has something to disambiguate.
func newSearchFixture() *MemStore {
	s := NewMemStore()
	addRegions(s,
		// States first (parents must exist before children).
		Region{ID: 1, Slug: "il", Kind: "us:state", Name: "Illinois", Country: CountryUS, ScopeTier: ScopeRegional},
		Region{ID: 2, Slug: "ma", Kind: "us:state", Name: "Massachusetts", Country: CountryUS, ScopeTier: ScopeRegional},
		Region{ID: 3, Slug: "ny", Kind: "us:state", Name: "New York", Country: CountryUS, ScopeTier: ScopeRegional},
		// Metro + borough leaf carrying the state edge directly.
		Region{ID: 4, Slug: "nyc-metro", Kind: "us:metro", Name: "New York Metro", Country: CountryUS, ScopeTier: ScopeRegional, ParentSlugs: []string{"ny"}},
		Region{ID: 5, Slug: "queens", Kind: "us:borough", Name: "Queens", Country: CountryUS, ScopeTier: ScopeLocal, ParentSlugs: []string{"nyc-metro", "ny"}},
		// Two same-named cities in different states.
		Region{ID: 6, Slug: "springfield-il", Kind: "us:city", Name: "Springfield", Country: CountryUS, ScopeTier: ScopeLocal, ParentSlugs: []string{"il"}},
		Region{ID: 7, Slug: "springfield-ma", Kind: "us:city", Name: "Springfield", Country: CountryUS, ScopeTier: ScopeLocal, ParentSlugs: []string{"ma"}},
		// A substring-only match (ranks below the exact-name hits).
		Region{ID: 8, Slug: "west-springfield-ma", Kind: "us:city", Name: "West Springfield", Country: CountryUS, ScopeTier: ScopeLocal, ParentSlugs: []string{"ma"}},
		// A national-tier row whose name matches "queens" — must never surface.
		Region{ID: 99, Slug: "queens-national", Kind: "xx:national", Name: "Queens National Network", Country: CountryUS, ScopeTier: ScopeNational},
	)
	return s
}

func resultSlugs(rs []RegionSearchResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Region.Slug
	}
	return out
}

func TestMemStore_SearchRegions_RanksExactSlugFirstAndExcludesNational(t *testing.T) {
	s := newSearchFixture()
	got, err := s.SearchRegions(context.Background(), "queens", 0)
	if err != nil {
		t.Fatalf("SearchRegions: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one result for 'queens'")
	}
	if got[0].Region.Slug != "queens" {
		t.Errorf("exact-slug match should rank first; got %q", got[0].Region.Slug)
	}
	for _, r := range got {
		if r.Region.ScopeTier == ScopeNational {
			t.Errorf("national-tier region %q must be excluded", r.Region.Slug)
		}
		if r.Region.Slug == "queens-national" {
			t.Errorf("national row leaked into search results")
		}
	}
}

func TestMemStore_SearchRegions_ContextLabelFromStateAncestor(t *testing.T) {
	s := newSearchFixture()
	got, err := s.SearchRegions(context.Background(), "queens", 0)
	if err != nil {
		t.Fatalf("SearchRegions: %v", err)
	}
	if got[0].ContextLabel != "New York" {
		t.Errorf("context label: want %q, got %q", "New York", got[0].ContextLabel)
	}
}

func TestMemStore_SearchRegions_ContextLabelFallsBackToDirectParent(t *testing.T) {
	// A metro reachable only through multi-state coalitions has no state
	// ancestor (us:multi-state is not a state kind), so regionContextLabel
	// can't satisfy the BFS and falls back to firstParentName: the
	// alphabetically-first (slug ASC) direct, non-national parent's name.
	// Parents are wired in reverse slug order so a passing result proves
	// the slug-ASC tiebreak, not insertion order.
	s := NewMemStore()
	addRegions(s,
		Region{ID: 1, Slug: "beta-coalition", Kind: "us:multi-state", Name: "Beta Coalition", Country: CountryUS, ScopeTier: ScopeRegional},
		Region{ID: 2, Slug: "alpha-coalition", Kind: "us:multi-state", Name: "Alpha Coalition", Country: CountryUS, ScopeTier: ScopeRegional},
		Region{ID: 3, Slug: "orphan-metro", Kind: "us:metro", Name: "Orphan Metro", Country: CountryUS, ScopeTier: ScopeRegional, ParentSlugs: []string{"beta-coalition", "alpha-coalition"}},
	)
	got, err := s.SearchRegions(context.Background(), "orphan", 0)
	if err != nil {
		t.Fatalf("SearchRegions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 result for 'orphan', got %d", len(got))
	}
	if got[0].ContextLabel != "Alpha Coalition" {
		t.Errorf("fallback context label: want %q (slug-ASC-first direct parent), got %q", "Alpha Coalition", got[0].ContextLabel)
	}
}

func TestMemStore_SearchRegions_DisambiguatesDuplicateNames(t *testing.T) {
	s := newSearchFixture()
	got, err := s.SearchRegions(context.Background(), "springfield", 0)
	if err != nil {
		t.Fatalf("SearchRegions: %v", err)
	}
	// Two exact-name "Springfield" hits rank ahead of the "West
	// Springfield" substring match; tiebreak Name ASC then Slug ASC puts
	// springfield-il before springfield-ma.
	wantOrder := []string{"springfield-il", "springfield-ma", "west-springfield-ma"}
	if diff := cmp.Diff(wantOrder, resultSlugs(got)); diff != "" {
		t.Errorf("ranking order (-want +got):\n%s", diff)
	}
	if got[0].ContextLabel != "Illinois" {
		t.Errorf("springfield-il context: want %q, got %q", "Illinois", got[0].ContextLabel)
	}
	if got[1].ContextLabel != "Massachusetts" {
		t.Errorf("springfield-ma context: want %q, got %q", "Massachusetts", got[1].ContextLabel)
	}
}

func TestMemStore_SearchRegions_EmptyQueryReturnsEmptySlice(t *testing.T) {
	s := newSearchFixture()
	for _, q := range []string{"", "   "} {
		got, err := s.SearchRegions(context.Background(), q, 0)
		if err != nil {
			t.Fatalf("SearchRegions(%q): %v", q, err)
		}
		if got == nil {
			t.Errorf("SearchRegions(%q): want non-nil empty slice, got nil", q)
		}
		if len(got) != 0 {
			t.Errorf("SearchRegions(%q): want empty, got %d results", q, len(got))
		}
	}
}

func TestMemStore_SearchRegions_LimitCapAndExplicitLimit(t *testing.T) {
	s := newSearchFixture()
	// Explicit small limit is honored.
	got, err := s.SearchRegions(context.Background(), "springfield", 1)
	if err != nil {
		t.Fatalf("SearchRegions: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("limit=1: want 1 result, got %d", len(got))
	}
	if got[0].Region.Slug != "springfield-il" {
		t.Errorf("limit=1 should keep the top-ranked hit; got %q", got[0].Region.Slug)
	}
	// A limit beyond the hard max is clamped — exercised via a tiny
	// fixture, so just assert it doesn't exceed the cap.
	all, err := s.SearchRegions(context.Background(), "springfield", 9999)
	if err != nil {
		t.Fatalf("SearchRegions: %v", err)
	}
	if len(all) > maxRegionSearchLimit {
		t.Errorf("result count %d exceeds hard cap %d", len(all), maxRegionSearchLimit)
	}
}
