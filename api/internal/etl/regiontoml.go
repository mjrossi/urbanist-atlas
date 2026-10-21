package etl

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// RegionRow is one [[region]] in a generated regions_*.toml: a metro/CMA
// umbrella or a per-state/-province portion. Comment is a full "# …"
// provenance line written after the row, or empty.
type RegionRow struct {
	Slug         string
	Name         string
	Kind         string
	Parents      []string
	RollupStates []string
	Comment      string
}

// PortionParent is one state or province an umbrella spans: its region
// slug and the bare abbrev used in portion slugs ("ca-state" → "ca").
type PortionParent struct {
	Slug   string
	Abbrev string
}

// BuildPortionRows returns one portion row per spanned parent, in
// spanned order, with slug "<umbrella>-<abbrev>", name
// "<Umbrella Name> (<ABBREV>)" and parents [parent slug, umbrella slug].
func BuildPortionRows(umbrellaSlug, umbrellaName, kind string, spanned []PortionParent) []RegionRow {
	rows := make([]RegionRow, 0, len(spanned))
	for _, p := range spanned {
		rows = append(rows, RegionRow{
			Slug:    umbrellaSlug + "-" + p.Abbrev,
			Name:    umbrellaName + " (" + strings.ToUpper(p.Abbrev) + ")",
			Kind:    kind,
			Parents: []string{p.Slug, umbrellaSlug},
		})
	}
	return rows
}

// WriteRegionsTOML writes header and then the rows sorted by slug, which
// puts each "<umbrella>-<abbrev>" portion after its umbrella, as the
// loader's parents-first order needs. Every generated row is
// scope_tier "regional", sort_priority 40.
func WriteRegionsTOML(w io.Writer, header string, rows []RegionRow) error {
	sorted := make([]RegionRow, len(rows))
	copy(sorted, rows)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Slug < sorted[j].Slug })

	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString(header); err != nil {
		return err
	}
	for _, r := range sorted {
		if r.Slug == "" {
			return fmt.Errorf("write regions: empty slug (name %q)", r.Name)
		}
		// No shared default kind can exist across countries.
		if r.Kind == "" {
			return fmt.Errorf("write regions: empty kind (slug %q)", r.Slug)
		}
		if _, err := fmt.Fprintf(bw, "\n[[region]]\nslug = %s\nkind = %s\nname = %s\nscope_tier = \"regional\"\nsort_priority = 40\nparents = [",
			tomlString(r.Slug), tomlString(r.Kind), tomlString(r.Name)); err != nil {
			return err
		}
		for i, p := range r.Parents {
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
		if err := writeStringList(bw, "rollup_states", r.RollupStates); err != nil {
			return err
		}
		if r.Comment != "" {
			if _, err := fmt.Fprintf(bw, "%s\n", r.Comment); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}

// writeStringList writes a `key = [...]` line, or nothing when items is
// empty, so rows without the field keep their bytes.
func writeStringList(w *bufio.Writer, key string, items []string) error {
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "%s = [", key); err != nil {
		return err
	}
	for i, it := range items {
		if i > 0 {
			if _, err := w.WriteString(", "); err != nil {
				return err
			}
		}
		if _, err := w.WriteString(tomlString(it)); err != nil {
			return err
		}
	}
	_, err := w.WriteString("]\n")
	return err
}

// tomlString wraps s in a TOML 1.0 basic string. Non-ASCII UTF-8 passes
// through, so "Montréal" stays readable instead of Go %q's \xXX escapes.
// TOML forbids raw U+0000–U+001F and U+007F in a basic string, so those
// become short escapes where the spec has one and \uXXXX otherwise.
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
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
