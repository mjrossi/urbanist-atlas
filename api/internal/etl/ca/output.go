package ca

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// CMAOverride is one editorial override read from
// api/seed/regions_ca_cma_overrides.toml. It pins the slug, display
// name, and kind for a specific StatsCan CMA (keyed by its 3-digit
// UID) so the auto-generated values can be replaced with the curated
// form (e.g., "metro-vancouver" / "ca:regional-district").
//
// Parent edges are deliberately NOT override-able: every CA CMA's
// parents are derived from its StatsCan ProvinceUIDs (see assignCMAs).
// The US side carries a parents override because multi-state metros
// reroute into intermediate colloquial regions (nyc-metro →
// nyc-tristate); Canada has no such multi-province intermediate-region
// layer in v1, so province derivation is always correct
// (Ottawa-Gatineau → [on, qc]). Rather than ship an unused knob, we
// omit the field — a future country (or CA itself) can add it the day
// a concrete editorial need appears.
//
// It mirrors the US MSAOverride struct (api/internal/etl/us/output.go)
// for slug/name/kind — both countries drive those editorial overrides
// from data, not compiled Go — with an added Kind field (Metro
// Vancouver overrides the "ca:cma" default to "ca:regional-district").
type CMAOverride struct {
	UID  string `toml:"cma_uid"`
	Slug string `toml:"slug"`
	Name string `toml:"name"`
	Kind string `toml:"kind"`
}

// CMAAssignment captures the per-CMA slug + kind + parents that the
// output writer emits to regions_ca_cmas.toml. Overrides take effect
// during assignment (see assignCMAs).
type CMAAssignment struct {
	UID     string
	Slug    string
	Kind    string
	Name    string
	Parents []string
	// RollupStates is the directional rollup_states list emitted onto the
	// region row (atlas.Region.RollupStates): the province slugs on whose
	// detail pages this CMA's OWN orgs should surface (browse direction
	// only). Set on multi-province umbrellas (Ottawa-Gatineau → on, qc);
	// empty for single-province CMAs and for portion rows.
	RollupStates []string
}

// assignCMAs produces one assignment per CMA in canonical order
// (sorted by slug). Override entries (keyed by CMA UID, read from
// api/seed/regions_ca_cma_overrides.toml) supply slug/name/kind
// overrides; missing fields fall back to auto-generated values
// (slug = "<slugified-name>-cma", kind = "ca:cma", name = StatsCan
// name). Parents are derived from ProvinceUIDs (single-province →
// [province slug]; multi-province like Ottawa-Gatineau → [primary,
// secondary]).
func assignCMAs(cmas []CMA, overrides []CMAOverride) []CMAAssignment {
	overrideByUID := make(map[string]CMAOverride, len(overrides))
	for _, o := range overrides {
		overrideByUID[o.UID] = o
	}
	out := make([]CMAAssignment, 0, len(cmas))
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
		// Multi-province CMA (Ottawa-Gatineau): STATELESS umbrella +
		// rollup_states, with per-province FSA routing via portions
		// (buildCMAPortions). Single-province CMAs keep their province
		// parent edge. Mirrors the US multi-state metro handling.
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
		out = append(out, CMAAssignment{
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

// provinceEntry is one province a CMA spans: the StatsCan PRUID, the
// province region slug (e.g. "on", "qc", "nl-province"), and the bare
// abbrev used in portion slugs/names (the slug minus "-province").
type provinceEntry struct {
	PRUID  string
	Slug   string
	Abbrev string
}

// spannedProvinces returns the distinct provinces a CMA spans, derived
// from its StatsCan ProvinceUIDs, sorted by slug for deterministic
// output. Unknown PRUIDs are skipped.
func spannedProvinces(c CMA) []provinceEntry {
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

// buildCMAPortions returns the per-province portion rows for multi-
// province CMAs (only Ottawa-Gatineau in v1) plus the portion anchor
// lookup ("umbrellaSlug:PRUID" → portion slug) the FSA crosswalk routes
// through. Mirrors us.BuildRegionRows' portion logic.
func buildCMAPortions(cmas []CMA, assignments []CMAAssignment) ([]CMAAssignment, map[string]string) {
	byUID := make(map[string]CMAAssignment, len(assignments))
	for _, a := range assignments {
		byUID[a.UID] = a
	}
	var portions []CMAAssignment
	portionByCMA := map[string]string{}
	for _, c := range cmas {
		provs := spannedProvinces(c)
		if len(provs) < 2 {
			continue
		}
		a := byUID[c.UID]
		for _, p := range provs {
			portionSlug := a.Slug + "-" + p.Abbrev
			portions = append(portions, CMAAssignment{
				Slug:    portionSlug,
				Kind:    "ca:cma-portion",
				Name:    a.Name + " (" + strings.ToUpper(p.Abbrev) + ")",
				Parents: []string{p.Slug, a.Slug},
			})
			portionByCMA[a.Slug+":"+p.PRUID] = portionSlug
		}
	}
	return portions, portionByCMA
}

// cmaRowsToRegionRows adapts the CA assignment shape to the shared
// etl.RegionRow the common TOML writer consumes. The provenance comment
// (# StatsCan CMA <UID> — <Name>) is attached only to umbrella rows
// (UID set); portion rows carry no UID and so no comment, matching the
// prior WriteCMAsTOML behavior.
func cmaRowsToRegionRows(assignments []CMAAssignment) []etl.RegionRow {
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
# Loaded by just loaddata BETWEEN regions_ca_provinces.toml (parents:
# provinces) and regions_ca.toml (children: curated cities). Cross-file
# parent resolution lives in internal/seedfiles/regions.go.
`

// WritePostalCodesCSV emits postal_codes_ca.csv deterministically:
// rows sorted by FSA ASC, LF line endings, trailing newline.
func WritePostalCodesCSV(w io.Writer, anchors []PostalAnchor) error {
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].FSA < anchors[j].FSA })
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"postal_code", "country", "leaf_region_slug"}); err != nil {
		return err
	}
	for _, a := range anchors {
		if err := cw.Write([]string{a.FSA, "CA", a.AnchorSlug}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
