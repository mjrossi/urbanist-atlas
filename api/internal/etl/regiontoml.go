package etl

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// RegionRow is one emitted [[region]] in a generated regions_*.toml
// file: a metro/CMA umbrella or an auto-generated per-state/-province
// portion. Comment, when set, is the full provenance line (including
// the leading '#', no trailing newline) emitted after the row —
// umbrellas carry one, portions don't. Both country plans populate this
// shape and hand it to WriteRegionsTOML.
type RegionRow struct {
	Slug         string
	Name         string
	Kind         string
	Parents      []string
	RollupStates []string
	Comment      string
}

// WriteRegionsTOML emits a generated regions_*.toml file
// deterministically: header, then one [[region]] block per row sorted
// by slug ASC (so a "<umbrella>-<sub>" portion lands right after its
// umbrella, satisfying the loader's parents-first order), no embedded
// timestamps, LF line endings, trailing newline. scope_tier and
// sort_priority are fixed regional/40 — every generated metro/CMA row
// is regional. Strings are escaped via tomlString so non-ASCII names
// (Montréal, Mayagüez) round-trip as valid TOML basic strings.
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

// writeStringList writes a TOML `key = ["a", "b"]` line (plus newline),
// but ONLY when items is non-empty — an empty list emits nothing, so
// region rows without the field stay byte-identical. Used for the
// optional rollup_states field.
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

// tomlString wraps s in TOML double-quoted basic-string syntax.
// Backslashes and double quotes are escaped; UTF-8 multibyte chars
// pass through unchanged (TOML basic strings accept any UTF-8), so a
// name like "Montréal" stays "Montréal" rather than the \xXX escapes
// Go's %q would emit. Control characters (newline, tab, …) follow the
// TOML spec and become \n / \t.
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
