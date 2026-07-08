package us

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// MSAOverride is one editorial override read from
// api/seed/regions_us_msa_overrides.toml. It pins the slug, display
// name, and parent edges for a specific CBSA so the auto-generated
// values (which produce verbose "city-state-metro" slugs) can be
// replaced with the curated form (e.g., "nyc-metro").
//
// There is no kind override: every US CBSA is editorially a us:metro,
// which BuildRegionRows sets and etl.WriteRegionsTOML emits. If a future
// CBSA ever needs a different kind, add a Kind field here then — until that concrete need
// exists, the symmetry with CA's CMAOverride (which does carry Kind, for
// Metro Vancouver → ca:regional-district) is deliberately left unbuilt
// per the project's no-preemptive-abstraction convention.
type MSAOverride struct {
	CBSACode string   `toml:"cbsa_code"`
	Slug     string   `toml:"slug"`
	Name     string   `toml:"name"`
	Parents  []string `toml:"parents"`
	// RollupStates is the directional rollup_states list emitted onto the
	// region row (atlas.Region.RollupStates): the state slugs on whose
	// detail pages this metro's OWN orgs should surface, browse-direction
	// only. Empty for nearly every metro; set on the curated multi-state
	// metros to the full spanned set (e.g. chicago-metro → ["il", "in"],
	// nyc-metro → ["nj", "ny"]), matching what the auto-gen path computes
	// for non-overridden multi-state metros.
	RollupStates []string `toml:"rollup_states"`
}

// AssignMSASlugs returns (slug, displayName, parents) for every MSA,
// keyed by CBSA code. Override entries win over auto-generated values.
// Auto-gen slug is "<first-city>-<primary-state>-metro" — the primary
// state suffix is always included so slugs stay stable across Census
// revisions that may add or remove MSAs sharing a first-city name.
func AssignMSASlugs(msas []MSA, overrides []MSAOverride) map[string]MSAOverride {
	overrideByCode := map[string]MSAOverride{}
	for _, o := range overrides {
		overrideByCode[o.CBSACode] = o
	}

	out := make(map[string]MSAOverride, len(msas))
	for _, m := range msas {
		if o, ok := overrideByCode[m.CBSACode]; ok {
			out[m.CBSACode] = o
			continue
		}
		a := MSAOverride{
			CBSACode: m.CBSACode,
			Slug:     autoSlug(m),
			Name:     autoName(m),
		}
		// Multi-state by constituent counties (authoritative — the title's
		// abbrev list can disagree): emit a STATELESS umbrella plus
		// rollup_states, so the metro's own orgs surface on each spanned
		// state's page (browse direction) WITHOUT leaking state-tier orgs
		// across the line in postal lookups (docs/region-graph.md §1).
		// Per-state ZCTA routing goes through the portions BuildRegionRows
		// generates. Single-state MSAs keep their title-based state parent
		// edge, unchanged.
		if states := spannedStates(m); len(states) >= 2 {
			a.Parents = []string{}
			a.RollupStates = make([]string, len(states))
			for i, s := range states {
				a.RollupStates[i] = s.Slug
			}
		} else {
			a.Parents = autoParents(m)
		}
		out[m.CBSACode] = a
	}
	return out
}

// stateEntry is one state an MSA's counties fall in: the 2-digit FIPS
// prefix, the region slug (e.g. "il", "ca-state", "dc"), and the bare
// 2-letter abbrev used in portion slugs/names (the slug minus "-state").
type stateEntry struct {
	FIPS   string
	Slug   string
	Abbrev string
}

// spannedStates returns the distinct states an MSA spans, derived from
// its constituent county FIPS (the authoritative source — a ZCTA always
// routes to a county that is genuinely in the MSA, so every routed state
// has a portion). Sorted by slug for deterministic output. Counties
// whose 2-digit prefix isn't a known state/territory/district FIPS are
// skipped.
func spannedStates(m MSA) []stateEntry {
	seen := map[string]bool{}
	var out []stateEntry
	for _, geoid := range m.Counties {
		if len(geoid) < 2 {
			continue
		}
		fips := geoid[:2]
		slug, ok := stateFIPSToSlug[fips]
		if !ok || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, stateEntry{FIPS: fips, Slug: slug, Abbrev: strings.TrimSuffix(slug, "-state")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// firstCity returns the leading city name in a CBSA title — everything
// up to (and not including) the first hyphen or comma. Handles single
// hyphens between cities (most CBSAs) and double-hyphens that the
// Census uses when a city itself contains a hyphen (e.g.
// "Scranton--Wilkes-Barre, PA").
func firstCity(title string) string {
	i := strings.IndexAny(title, "-,")
	if i < 0 {
		return strings.TrimSpace(title)
	}
	return strings.TrimSpace(title[:i])
}

func autoSlug(m MSA) string {
	city := etl.Slugify(firstCity(m.Title))
	if city == "" {
		// Defensive: fall back to CBSA code so we always have *some*
		// unique slug rather than blowing up the load.
		return "msa-" + m.CBSACode
	}
	state := ""
	if len(m.StateAbbrevs) > 0 {
		state = strings.ToLower(m.StateAbbrevs[0])
	}
	if state == "" {
		return city + "-metro"
	}
	return city + "-" + state + "-metro"
}

func autoName(m MSA) string {
	city := firstCity(m.Title)
	if city == "" {
		return m.Title
	}
	return city + " Metro"
}

// autoParents returns the state-slug parents for a single-state MSA
// (the only case AssignMSASlugs calls it — multi-state MSAs are stateless
// umbrellas). The state in the title contributes a parent edge so
// MSA-anchored ZCTAs reach their state-tier orgs via the ancestor walk.
// For the curated multi-state metros (NYC, Chicago) the override file
// supersedes this with the advocacy-node parent (nyc-tristate /
// chicagoland); their per-state reachability comes from the portions
// BuildRegionRows generates, same as every other multi-state metro.
func autoParents(m MSA) []string {
	parents := make([]string, 0, len(m.StateAbbrevs))
	seen := map[string]bool{}
	for _, abbrev := range m.StateAbbrevs {
		slug, ok := statePostalToSlug[abbrev]
		if !ok || seen[slug] {
			continue
		}
		parents = append(parents, slug)
		seen[slug] = true
	}
	return parents
}

// RegionRow aliases the shared etl.RegionRow. A US row is one emitted
// [[region]] in regions_us_msas.toml: a metro umbrella (Kind us:metro)
// or an auto-generated per-state portion (Kind us:metro-portion).
// Comment is the trailing "# Census CBSA …" line for umbrellas, empty
// for portions.
type RegionRow = etl.RegionRow

// BuildRegionRows expands per-CBSA assignments into the full set of
// emitted region rows — one umbrella per MSA, plus one us:metro-portion
// per spanned state for every multi-state MSA — and the portion anchor
// lookup (cbsaCode+":"+stateFIPS → portion slug) the crosswalk uses to
// anchor each ZCTA to its OWN state's portion.
//
// There is no flagship special-case: the curated multi-state metros
// (nyc-metro, chicago-metro, greater-boston) generate portions exactly
// like every auto-generated multi-state metro (docs/region-graph.md §1).
// Their override only supplies the curated slug/name and — for the
// advocacy-node flagships — the umbrella parent (nyc-tristate /
// chicagoland); the portions + rollup_states come from the spanned
// states. Curated borough/county leaves still win as the smaller anchor
// (county_resolver tiers 1-2), so only ZIPs lacking a curated leaf
// re-anchor at a portion. Single-state MSAs (overridden or not) span <2
// states and get no portion.
func BuildRegionRows(msas []MSA, assignments map[string]MSAOverride) ([]RegionRow, map[string]string) {
	var rows []RegionRow
	portionSlugs := map[string]string{}
	for _, m := range msas {
		a := assignments[m.CBSACode]
		rows = append(rows, RegionRow{
			Slug:         a.Slug,
			Name:         a.Name,
			Kind:         "us:metro",
			Parents:      a.Parents,
			RollupStates: a.RollupStates,
			Comment:      fmt.Sprintf("# Census CBSA %s — %s", m.CBSACode, m.Title),
		})
		states := spannedStates(m)
		if len(states) < 2 {
			continue
		}
		spanned := make([]etl.PortionParent, len(states))
		for i, s := range states {
			spanned[i] = etl.PortionParent{Slug: s.Slug, Abbrev: s.Abbrev}
		}
		portions := etl.BuildPortionRows(a.Slug, a.Name, "us:metro-portion", spanned)
		rows = append(rows, portions...)
		for i, s := range states {
			portionSlugs[m.CBSACode+":"+s.FIPS] = portions[i].Slug
		}
	}
	return rows, portionSlugs
}

const msaTOMLHeader = `# US Metropolitan Statistical Areas (MSAs), generated from the
# Census Bureau's CBSA delineation file by api/cmd/server etl
# regenerate --country=US.
#
# Edit policy: do NOT hand-edit this file. Editorial overrides for
# slug, name, and parents live in regions_us_msa_overrides.toml and
# are applied by the ETL run. To change a metro's slug/name/parents,
# add or update an override and re-run etl regenerate.
#
# Slug convention:
#   - Override-supplied for the well-known metros (nyc-metro,
#     chicago-metro, etc.). See regions_us_msa_overrides.toml.
#   - Auto-generated otherwise as "<first-city>-<primary-state>-metro"
#     (the state suffix keeps slugs stable across Census revisions
#     that may shuffle MSAs sharing a first-city name).
#
# Parents:
#   - Single-state MSAs parent under their state slug.
#   - Multi-state MSAs are STATELESS umbrellas: parents = [] plus
#     rollup_states = [each spanned state], so the metro's own orgs
#     surface on each state's detail page (browse direction) WITHOUT
#     leaking state-tier orgs across the line in postal lookups. Each
#     spanned state also gets a us:metro-portion leaf
#     (parents = [state, umbrella]) that its constituent ZCTAs anchor at,
#     so a lookup reaches only its own state. The portion slug is
#     "<umbrella>-<state>", which sorts right after the umbrella.
#   - The curated multi-state flagships (nyc-metro, chicago-metro) follow
#     the SAME portion model; their only override is the curated slug/name
#     and an advocacy-node umbrella parent (nyc-tristate / chicagoland)
#     in place of the empty parent. Hand-curated borough/county leaves
#     still win as the smaller anchor, so only ZIPs lacking a curated leaf
#     ride the portions.
#
# Loaded at API boot by internal/seedfiles into the in-memory
# FileStore, BETWEEN regions_us_states.toml (parents: states) and
# regions_us.toml (children: curated city/borough leaves that may
# reference these metros). Cross-file parent resolution lives in
# internal/seedfiles/regions.go.
`

// WritePostalCodesCSV emits the postal_codes_us.csv file
// deterministically. It merges the two anchor sources — the primary
// ZCTA pass (slice #7.5.3) and the HUD non-ZCTA backfill (slice #7.5.5)
// — with ZCTA winning any (country, postal_code) tie, then hands the
// deduped set to the shared etl.WritePostalCSV writer (sorted by
// postal_code ASC, LF endings, trailing newline). Pass nil or an empty
// slice for hudAnchors when running without HUD data; the output
// reduces to ZCTA-only in that case, matching the pre-#7.5.5 behavior.
func WritePostalCodesCSV(w io.Writer, zctaAnchors, hudAnchors []PostalAnchor) error {
	// Build a dedup map keyed by postal code; ZCTA inserted first so
	// HUD rows with the same key are silently dropped at insertion.
	merged := make(map[string]PostalAnchor, len(zctaAnchors)+len(hudAnchors))
	for _, a := range zctaAnchors {
		merged[a.PostalCode] = a
	}
	for _, a := range hudAnchors {
		if _, ok := merged[a.PostalCode]; ok {
			continue
		}
		merged[a.PostalCode] = a
	}

	deduped := make([]PostalAnchor, 0, len(merged))
	for _, a := range merged {
		deduped = append(deduped, a)
	}
	return etl.WritePostalCSV(w, "US", deduped)
}
