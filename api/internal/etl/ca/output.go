package ca

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// CMAOverride is one editorial override read from
// api/seed/regions_ca_cma_overrides.toml. It pins the slug, display
// name, kind, and parent edges for a specific StatsCan CMA (keyed by
// its 3-digit UID) so the auto-generated values can be replaced with
// the curated form (e.g., "metro-vancouver" / "ca:regional-district").
//
// It mirrors the US MSAOverride struct (api/internal/etl/us/output.go)
// — both countries now drive editorial overrides from data, not
// compiled Go — with an added Kind field (Metro Vancouver overrides
// the "ca:cma" default to "ca:regional-district").
type CMAOverride struct {
	UID     string   `toml:"cma_uid"`
	Slug    string   `toml:"slug"`
	Name    string   `toml:"name"`
	Kind    string   `toml:"kind"`
	Parents []string `toml:"parents"`
}

type overrideFile struct {
	Overrides []CMAOverride `toml:"override"`
}

// ReadCMAOverrides parses the overrides TOML file. Missing file is
// not an error — the file is optional, and the auto-gen flow has a
// sensible default for every CMA. Mirrors us.ReadMSAOverrides.
func ReadCMAOverrides(path string) ([]CMAOverride, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read overrides: %w", err)
	}
	var f overrideFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse overrides: %w", err)
	}
	return f.Overrides, nil
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
# slug/name/kind/parents live in regions_ca_cma_overrides.toml (keyed
# by StatsCan CMA UID); change those and re-run etl regenerate.
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
