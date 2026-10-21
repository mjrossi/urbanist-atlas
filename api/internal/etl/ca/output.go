package ca

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// CMAOverride is one editorial override from
// api/seed/regions_ca_cma_overrides.toml, pinning a CMA's slug, name and
// kind (e.g. Metro Vancouver as "ca:regional-district"). There is no
// parents override: CA has no multi-province intermediate regions, so
// parents always derive from the CMA's provinces.
type CMAOverride struct {
	UID  string `toml:"cma_uid"`
	Slug string `toml:"slug"`
	Name string `toml:"name"`
	Kind string `toml:"kind"`
}

// cmaAssignment is the slug, kind, name and parents emitted for one CMA.
type cmaAssignment struct {
	UID     string
	Slug    string
	Kind    string
	Name    string
	Parents []string
	// RollupStates lists the province slugs on whose detail pages this
	// CMA's own orgs surface (browse direction only); set only on
	// multi-province umbrellas.
	RollupStates []string
}

// assignCMAs returns one assignment per CMA, sorted by slug. Overrides
// supply slug, name and kind, defaulting to "<slugified-name>-cma",
// the StatsCan name and "ca:cma". A single-province CMA parents under
// its province; a multi-province one gets no parents and rollup_states.
func assignCMAs(cmas []cma, overrides []CMAOverride) []cmaAssignment {
	overrideByUID := make(map[string]CMAOverride, len(overrides))
	for _, o := range overrides {
		overrideByUID[o.UID] = o
	}
	out := make([]cmaAssignment, 0, len(cmas))
	for _, c := range cmas {
		override := overrideByUID[c.UID]
		slug := override.Slug
		if slug == "" {
			slug = etl.Slugify(c.Name) + "-cma"
		}
		name := override.Name
		if name == "" {
			name = c.Name
		}
		kind := override.Kind
		if kind == "" {
			kind = "ca:cma"
		}
		// Same stateless-umbrella model as US multi-state metros: FSAs route
		// through per-province portions (buildCMAPortions).
		provs := spannedProvinces(c)
		parents := []string{}
		var rollup []string
		if len(provs) >= 2 {
			rollup = make([]string, len(provs))
			for i, p := range provs {
				rollup[i] = p.Slug
			}
		} else {
			for _, p := range provs {
				parents = append(parents, p.Slug)
			}
		}
		out = append(out, cmaAssignment{
			UID:          c.UID,
			Slug:         slug,
			Kind:         kind,
			Name:         name,
			Parents:      parents,
			RollupStates: rollup,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// provinceEntry is one province a CMA spans: its PRUID, region slug, and
// the bare abbrev used in portion slugs (the slug minus "-province").
type provinceEntry struct {
	PRUID  string
	Slug   string
	Abbrev string
}

// spannedProvinces returns the distinct known provinces a CMA spans,
// sorted by slug.
func spannedProvinces(c cma) []provinceEntry {
	seen := map[string]bool{}
	var out []provinceEntry
	for _, pruid := range c.ProvinceUIDs {
		slug := provinceUIDToSlug[pruid]
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, provinceEntry{PRUID: pruid, Slug: slug, Abbrev: strings.TrimSuffix(slug, "-province")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// buildCMAPortions returns the portion rows for multi-province CMAs plus
// the "umbrellaSlug:PRUID" → portion slug lookup the FSA crosswalk
// routes through.
func buildCMAPortions(cmas []cma, assignments []cmaAssignment) ([]etl.RegionRow, map[string]string) {
	byUID := make(map[string]cmaAssignment, len(assignments))
	for _, a := range assignments {
		byUID[a.UID] = a
	}
	var portions []etl.RegionRow
	portionByCMA := map[string]string{}
	for _, c := range cmas {
		provs := spannedProvinces(c)
		if len(provs) < 2 {
			continue
		}
		a := byUID[c.UID]
		spanned := make([]etl.PortionParent, len(provs))
		for i, p := range provs {
			spanned[i] = etl.PortionParent{Slug: p.Slug, Abbrev: p.Abbrev}
		}
		rows := etl.BuildPortionRows(a.Slug, a.Name, "ca:cma-portion", spanned)
		portions = append(portions, rows...)
		for i, p := range provs {
			portionByCMA[a.Slug+":"+p.PRUID] = rows[i].Slug
		}
	}
	return portions, portionByCMA
}

// cmaRowsToRegionRows converts umbrella assignments to etl.RegionRow,
// adding the "# StatsCan CMA <UID> — <Name>" provenance comment.
func cmaRowsToRegionRows(assignments []cmaAssignment) []etl.RegionRow {
	rows := make([]etl.RegionRow, len(assignments))
	for i, a := range assignments {
		comment := ""
		if a.UID != "" {
			comment = fmt.Sprintf("# StatsCan CMA %s — %s", a.UID, a.Name)
		}
		rows[i] = etl.RegionRow{
			Slug:         a.Slug,
			Name:         a.Name,
			Kind:         a.Kind,
			Parents:      a.Parents,
			RollupStates: a.RollupStates,
			Comment:      comment,
		}
	}
	return rows
}

const cmaTOMLHeader = `# Canadian Census Metropolitan Areas (CMAs), generated from the
# Statistics Canada CMA boundary file by api/cmd/server etl
# regenerate --country=CA.
#
# Edit policy: do NOT hand-edit this file. Editorial overrides for
# slug/name/kind live in regions_ca_cma_overrides.toml (keyed by
# StatsCan CMA UID); change those and re-run etl regenerate.
#
# Filtering: only CMATYPE='B' rows from the StatsCan boundary file
# (true Census Metropolitan Areas, population ≥100k) are emitted.
# Census Agglomerations (CMATYPE='D') are dropped.
#
# Slug convention:
#   - Override-supplied for the well-known CMAs (toronto-cma,
#     montreal-cma, metro-vancouver). See regions_ca_cma_overrides.toml.
#   - Auto-generated otherwise as "<slugified-name>-cma".
#
# Parents:
#   - Single-province CMAs parent under their province slug.
#   - Multi-province CMAs (Ottawa-Gatineau) are STATELESS umbrellas:
#     parents = [] plus rollup_states = [each spanned province], so the
#     CMA's own orgs surface on each province's detail page (browse
#     direction) WITHOUT leaking province-tier orgs across the line in
#     postal lookups. Each spanned province also gets a ca:cma-portion
#     leaf (parents = [province, umbrella]) that its FSAs anchor at, so a
#     lookup reaches only its own province. Portion slug is
#     "<umbrella>-<province>", which sorts right after the umbrella.
#
# Loaded at API boot by internal/seedfiles into the in-memory
# FileStore, BETWEEN regions_ca_provinces.toml (parents: provinces)
# and regions_ca.toml (children: curated cities). Cross-file parent
# resolution lives in internal/seedfiles/regions.go.
`
