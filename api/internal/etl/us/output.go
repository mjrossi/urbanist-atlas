package us

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
)

// MSAOverride is one editorial override read from
// api/seed/regions_us_msa_overrides.toml. It pins the slug, display
// name, and parent edges for a specific CBSA so the auto-generated
// values (which produce verbose "city-state-metro" slugs) can be
// replaced with the curated form (e.g., "nyc-metro").
//
// There is no kind override: every US CBSA is editorially a us:metro,
// which WriteMSAsTOML emits as a literal. If a future CBSA ever needs a
// different kind, add a Kind field here then — until that concrete need
// exists, the symmetry with CA's CMAOverride (which does carry Kind, for
// Metro Vancouver → ca:regional-district) is deliberately left unbuilt
// per the project's no-preemptive-abstraction convention.
type MSAOverride struct {
	CBSACode string   `toml:"cbsa_code"`
	Slug     string   `toml:"slug"`
	Name     string   `toml:"name"`
	Parents  []string `toml:"parents"`
}

type overrideFile struct {
	Overrides []MSAOverride `toml:"override"`
}

// ReadMSAOverrides parses the overrides TOML file. Missing file is
// not an error — the file is optional, and the auto-gen flow has a
// sensible default for every MSA.
func ReadMSAOverrides(path string) ([]MSAOverride, error) {
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
		slug := autoSlug(m)
		name := autoName(m)
		parents := autoParents(m)
		out[m.CBSACode] = MSAOverride{
			CBSACode: m.CBSACode,
			Slug:     slug,
			Name:     name,
			Parents:  parents,
		}
	}
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

// slugify lowercases, drops diacritics, removes punctuation, and
// collapses whitespace + interior hyphens into single hyphens. Used
// for MSA slug generation; doesn't try to match every potential
// Unicode source — just the common Latin diacritics that appear in
// PR/Mayagüez-style CBSA titles.
func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		r = unicode.ToLower(r)
		// Fold common Latin diacritics: ü→u, é→e, etc. The diacritic
		// is dropped; the base letter is kept.
		if r >= 0x00C0 && r <= 0x017F {
			r = foldDiacritic(r)
		}
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
		// All other runes (punctuation, etc.) are dropped silently.
	}
	out := b.String()
	return strings.TrimRight(out, "-")
}

// foldDiacritic maps the Latin-1 Supplement + Latin Extended-A range
// to ASCII letters. Coverage is good enough for the few diacritics
// that appear in CBSA titles (mostly Spanish in PR rows).
func foldDiacritic(r rune) rune {
	switch r {
	case 'à', 'á', 'â', 'ã', 'ä', 'å', 'ā', 'ă', 'ą':
		return 'a'
	case 'è', 'é', 'ê', 'ë', 'ē', 'ĕ', 'ė', 'ę', 'ě':
		return 'e'
	case 'ì', 'í', 'î', 'ï', 'ĩ', 'ī', 'ĭ', 'į':
		return 'i'
	case 'ñ', 'ń', 'ņ', 'ň':
		return 'n'
	case 'ò', 'ó', 'ô', 'õ', 'ö', 'ø', 'ō', 'ŏ', 'ő':
		return 'o'
	case 'ù', 'ú', 'û', 'ü', 'ũ', 'ū', 'ŭ', 'ů', 'ű', 'ų':
		return 'u'
	case 'ÿ', 'ý':
		return 'y'
	case 'ç', 'ć', 'ĉ', 'ċ', 'č':
		return 'c'
	case 'ł', 'ļ', 'ľ':
		return 'l'
	case 'ś', 'ŝ', 'ş', 'š':
		return 's'
	case 'ź', 'ż', 'ž':
		return 'z'
	}
	return r
}

func autoSlug(m MSA) string {
	city := slugify(firstCity(m.Title))
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

// autoParents returns the state-slug parents for a non-overridden MSA.
// All states in the title contribute a parent edge so MSA-anchored
// ZCTAs reach their state-tier orgs via the ancestor walk. For
// well-known multi-state metros (NYC, Chicago) the override file
// supersedes this with an intermediate multi-state region parent.
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

// WriteMSAsTOML emits the regions_us_msas.toml file deterministically:
// regions sorted by slug ASC, no embedded timestamps, LF line endings,
// trailing newline.
func WriteMSAsTOML(w io.Writer, msas []MSA, assignments map[string]MSAOverride) error {
	sorted := make([]MSA, len(msas))
	copy(sorted, msas)
	// Sort by assigned slug so the file order is human-friendly.
	sort.Slice(sorted, func(i, j int) bool {
		return assignments[sorted[i].CBSACode].Slug < assignments[sorted[j].CBSACode].Slug
	})

	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString(msaTOMLHeader); err != nil {
		return err
	}
	for _, m := range sorted {
		a := assignments[m.CBSACode]
		if a.Slug == "" {
			return fmt.Errorf("write msas: empty slug for cbsa %s (%s)", m.CBSACode, m.Title)
		}
		if _, err := fmt.Fprintf(bw, "\n[[region]]\nslug = %q\nkind = \"us:metro\"\nname = %q\nscope_tier = \"regional\"\nsort_priority = 40\nparents = [", a.Slug, a.Name); err != nil {
			return err
		}
		for i, p := range a.Parents {
			if i > 0 {
				if _, err := bw.WriteString(", "); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(bw, "%q", p); err != nil {
				return err
			}
		}
		if _, err := bw.WriteString("]\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(bw, "# Census CBSA %s — %s\n", m.CBSACode, m.Title); err != nil {
			return err
		}
	}
	return bw.Flush()
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
#   - Multi-state MSAs without an override parent under all their
#     constituent states (so MSA-anchored ZCTAs surface state-tier
#     orgs through the ancestor walk). Curated multi-state regions
#     in regions_us.toml (nyc-tristate, chicagoland) are
#     plumbed in via the override file.
#
# Loaded by just loaddata BETWEEN regions_us_states.toml (parents:
# states) and regions_us.toml (children: curated city/borough leaves
# that may reference these metros). Cross-file parent resolution lives
# in internal/seedfiles/regions.go.
`

// WritePostalCodesCSV emits the postal_codes_us.csv file
// deterministically: rows sorted by postal_code ASC, LF line endings,
// trailing newline. Accepts two anchor sources — the primary ZCTA
// pass (slice #7.5.3) and the HUD non-ZCTA backfill (slice #7.5.5) —
// and merges them with ZCTA winning any (country, postal_code) tie.
// Pass nil or an empty slice for hudAnchors when running without HUD
// data; the output reduces to ZCTA-only in that case, matching the
// pre-#7.5.5 behavior.
func WritePostalCodesCSV(w io.Writer, zctaAnchors, hudAnchors []PostalAnchor) error {
	// Build a dedup map keyed by postal code; ZCTA inserted first so
	// HUD rows with the same key are silently dropped at insertion.
	merged := make(map[string]PostalAnchor, len(zctaAnchors)+len(hudAnchors))
	for _, a := range zctaAnchors {
		merged[a.ZCTA] = a
	}
	for _, a := range hudAnchors {
		if _, ok := merged[a.ZCTA]; ok {
			continue
		}
		merged[a.ZCTA] = a
	}

	zips := make([]string, 0, len(merged))
	for z := range merged {
		zips = append(zips, z)
	}
	sort.Strings(zips)

	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"postal_code", "country", "leaf_region_slug"}); err != nil {
		return err
	}
	for _, z := range zips {
		a := merged[z]
		if err := cw.Write([]string{a.ZCTA, "US", a.AnchorSlug}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
