package ca

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
)

// CMAAssignment captures the per-CMA slug + kind + parents that the
// output writer emits to regions_ca_cmas.toml. Overrides take effect
// during assignment (see assignCMAs).
type CMAAssignment struct {
	UID     string
	Slug    string
	Kind    string
	Name    string
	Parents []string
}

// assignCMAs produces one assignment per CMA in canonical order
// (sorted by slug). Entries in cmaOverrides (keyed by CMA UID)
// supply slug/name/kind overrides; missing fields fall back to
// auto-generated values (slug = "<slugified-name>-cma", kind =
// "ca:cma", name = StatsCan name). Parents are derived from
// ProvinceUIDs (single-province → [province slug]; multi-province
// like Ottawa-Gatineau → [primary, secondary]).
func assignCMAs(cmas []CMA) []CMAAssignment {
	out := make([]CMAAssignment, 0, len(cmas))
	for _, c := range cmas {
		override := cmaOverrides[c.UID]
		slug := override.Slug
		if slug == "" {
			slug = slugify(c.Name) + "-cma"
		}
		name := override.Name
		if name == "" {
			name = c.Name
		}
		kind := override.Kind
		if kind == "" {
			kind = "ca:cma"
		}
		parents := make([]string, 0, len(c.ProvinceUIDs))
		seen := map[string]bool{}
		for _, pruid := range c.ProvinceUIDs {
			ps := provinceUIDToSlug[pruid]
			if ps == "" || seen[ps] {
				continue
			}
			parents = append(parents, ps)
			seen[ps] = true
		}
		out = append(out, CMAAssignment{
			UID:     c.UID,
			Slug:    slug,
			Kind:    kind,
			Name:    name,
			Parents: parents,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// slugify is a small Latin-1-diacritic-folding slugger. CA CMAs have
// French names with é/è/à etc.; this strips them to ASCII so slugs
// match the project's "lowercase ASCII with hyphens" convention.
func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
			prevDash = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			if folded := foldDiacritic(r); folded != r {
				b.WriteRune(folded)
				prevDash = false
			}
			// other punctuation dropped silently
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func foldDiacritic(r rune) rune {
	switch r {
	case 'à', 'á', 'â', 'ã', 'ä', 'å':
		return 'a'
	case 'À', 'Á', 'Â', 'Ã', 'Ä', 'Å':
		return 'a'
	case 'è', 'é', 'ê', 'ë':
		return 'e'
	case 'È', 'É', 'Ê', 'Ë':
		return 'e'
	case 'ì', 'í', 'î', 'ï':
		return 'i'
	case 'Ì', 'Í', 'Î', 'Ï':
		return 'i'
	case 'ñ':
		return 'n'
	case 'Ñ':
		return 'n'
	case 'ò', 'ó', 'ô', 'õ', 'ö', 'ø':
		return 'o'
	case 'Ò', 'Ó', 'Ô', 'Õ', 'Ö', 'Ø':
		return 'o'
	case 'ù', 'ú', 'û', 'ü':
		return 'u'
	case 'Ù', 'Ú', 'Û', 'Ü':
		return 'u'
	case 'ç':
		return 'c'
	case 'Ç':
		return 'c'
	}
	return r
}

// WriteCMAsTOML emits regions_ca_cmas.toml deterministically.
func WriteCMAsTOML(w io.Writer, assignments []CMAAssignment) error {
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString(cmaTOMLHeader); err != nil {
		return err
	}
	for _, a := range assignments {
		// TOML basic strings allow arbitrary UTF-8 — we just need to
		// escape \ and " — so non-ASCII characters in CMA names (é, è
		// in Montréal/Trois-Rivières/Québec) round-trip cleanly. Using
		// Go's %q here would emit \xXX escapes that aren't valid TOML.
		if _, err := fmt.Fprintf(bw, "\n[[region]]\nslug = %s\nkind = %s\nname = %s\nscope_tier = \"regional\"\nsort_priority = 40\nparents = [",
			tomlString(a.Slug), tomlString(a.Kind), tomlString(a.Name)); err != nil {
			return err
		}
		for i, p := range a.Parents {
			if i > 0 {
				if _, err := bw.WriteString(", "); err != nil {
					return err
				}
			}
			if _, err := bw.WriteString(tomlString(p)); err != nil {
				return err
			}
		}
		if _, err := bw.WriteString("]\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(bw, "# StatsCan CMA %s — %s\n", a.UID, a.Name); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// tomlString wraps s in TOML double-quoted basic-string syntax.
// Backslashes and double quotes are escaped; UTF-8 multibyte chars
// pass through unchanged (TOML basic strings accept any UTF-8). The
// control-character handling (newline, tab, etc.) follows the TOML
// spec — those become \n / \t. We don't emit \xXX or \uXXXX escapes
// since the CMA names don't contain anything weirder than Latin-1.
func tomlString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

const cmaTOMLHeader = `# Canadian Census Metropolitan Areas (CMAs), generated from the
# Statistics Canada CMA boundary file by api/cmd/server etl
# regenerate --country=CA.
#
# Edit policy: do NOT hand-edit this file. Editorial overrides for
# slug/name/kind live in api/internal/etl/ca/mappings.go (cmaOverrides,
# keyed by StatsCan CMA UID); change those and re-run etl regenerate.
#
# Filtering: only CMATYPE='B' rows from the StatsCan boundary file
# (true Census Metropolitan Areas, population ≥100k) are emitted.
# Census Agglomerations (CMATYPE='D') are dropped.
#
# Slug convention:
#   - Override-supplied for the well-known CMAs (toronto-cma,
#     montreal-cma, metro-vancouver). See ca/mappings.go.
#   - Auto-generated otherwise as "<slugified-name>-cma".
#
# Parents:
#   - Single-province CMAs parent under their province slug.
#   - Multi-province CMAs (Ottawa-Gatineau) parent under all their
#     constituent provinces so MSA-anchored FSAs surface
#     province-tier orgs through the ancestor walk.
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
