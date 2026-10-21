package us

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// MSAOverride is one editorial override from
// api/seed/regions_us_msa_overrides.toml, pinning a CBSA's slug, name
// and parents in place of the verbose auto-generated values. Unlike CA's
// CMAOverride it has no Kind: every US CBSA is a us:metro.
type MSAOverride struct {
	CBSACode string   `toml:"cbsa_code"`
	Slug     string   `toml:"slug"`
	Name     string   `toml:"name"`
	Parents  []string `toml:"parents"`
	// RollupStates lists the state slugs on whose detail pages this
	// metro's own orgs surface (browse direction only), e.g. ["il", "in"]
	// for chicago-metro.
	RollupStates []string `toml:"rollup_states"`
}

// assignMSASlugs returns the slug, name and parents for every MSA, keyed
// by CBSA code, with overrides winning. The auto-generated slug always
// carries the primary state so it stays stable when Census adds or drops
// an MSA sharing the first city's name.
func assignMSASlugs(msas []msa, overrides []MSAOverride) map[string]MSAOverride {
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
		// Constituent counties, not the title's state list, decide
		// multi-state, because the CBSA title can name different states.
		// A multi-state MSA becomes a stateless umbrella with rollup_states so
		// its orgs surface on each state's page without leaking state-tier orgs
		// across the line in postal lookups (docs/region-graph.md §1).
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

// spannedStates returns the distinct states an MSA's counties fall in,
// sorted by slug. Counties whose FIPS prefix isn't a known state are
// skipped. A ZCTA always routes to a county inside its MSA, so every
// state a ZCTA can route to gets a portion.
func spannedStates(m msa) []stateEntry {
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

// firstCity returns a CBSA title up to the first hyphen or comma. Census
// writes a double hyphen when a city name contains one
// ("Scranton--Wilkes-Barre, PA"), so the first hyphen still ends the city.
func firstCity(title string) string {
	i := strings.IndexAny(title, "-,")
	if i < 0 {
		return strings.TrimSpace(title)
	}
	return strings.TrimSpace(title[:i])
}

func autoSlug(m msa) string {
	city := etl.Slugify(firstCity(m.Title))
	if city == "" {
		// Fall back to the CBSA code rather than fail the load.
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

func autoName(m msa) string {
	city := firstCity(m.Title)
	if city == "" {
		return m.Title
	}
	return city + " Metro"
}

// autoParents returns the state slugs named in a single-state MSA's
// title, so MSA-anchored ZCTAs reach their state-tier orgs.
func autoParents(m msa) []string {
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

// RegionRow aliases the shared etl.RegionRow. A US row is a metro
// umbrella (us:metro) or a generated per-state portion (us:metro-portion).
type RegionRow = etl.RegionRow

// buildRegionRows expands the per-CBSA assignments into region rows: one
// umbrella per MSA plus one us:metro-portion per spanned state of a
// multi-state MSA. It also returns the "cbsaCode:stateFIPS" → portion
// slug lookup that anchors each ZCTA at its own state's portion. Curated
// multi-state metros get portions like any other
// (docs/region-graph.md §1).
func buildRegionRows(msas []msa, assignments map[string]MSAOverride) ([]RegionRow, map[string]string) {
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

// writePostalCodesCSV merges the ZCTA and HUD anchors, ZCTA winning on a
// shared postal code, and writes postal_codes_us.csv. hudAnchors may be
// empty.
func writePostalCodesCSV(w io.Writer, zctaAnchors, hudAnchors []PostalAnchor) error {
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
