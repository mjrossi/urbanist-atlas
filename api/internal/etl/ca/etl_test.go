package ca

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Toronto", "toronto"},
		{"Montréal", "montreal"},
		{"Trois-Rivières", "trois-rivieres"},
		{"Québec", "quebec"},
		{"Ottawa - Gatineau", "ottawa-gatineau"},
		{"   leading/trailing///   ", "leading-trailing"},
		// Underscores treated as separators too.
		{"foo_bar", "foo-bar"},
		// Empty input.
		{"", ""},
		// Punctuation dropped silently (no separator emitted).
		{"O'Brien", "obrien"},
	}
	for _, c := range cases {
		got := slugify(c.in)
		if got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFoldDiacritic(t *testing.T) {
	// Spot-check the French CMA-name set: é/è/ê for Québec/Montréal,
	// à for à-prefixed cities (rare in CMAs but parsed elsewhere), and
	// the cedilla.
	cases := []struct {
		in, want rune
	}{
		{'é', 'e'}, {'è', 'e'}, {'ê', 'e'},
		{'à', 'a'}, {'â', 'a'},
		{'î', 'i'},
		{'ô', 'o'}, {'ö', 'o'},
		{'û', 'u'}, {'ù', 'u'},
		{'ç', 'c'},
		{'ñ', 'n'},
		// Unmapped passes through.
		{'z', 'z'},
	}
	for _, c := range cases {
		if got := foldDiacritic(c.in); got != c.want {
			t.Errorf("foldDiacritic(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCleanCMAName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// English/French split keeps first variant.
		{"Greater Sudbury / Grand Sudbury", "Greater Sudbury"},
		{"Ottawa - Gatineau (Ontario part / partie de l'Ontario)", "Ottawa - Gatineau"},
		{"Québec", "Québec"},
		// Parenthetical-only suffix.
		{"Halifax (CMA)", "Halifax"},
		// Both: French slash inside parens — slash check runs second,
		// but parens are stripped first so the slash never appears.
		{"Foo (en / fr)", "Foo"},
		// No transforms needed.
		{"Toronto", "Toronto"},
	}
	for _, c := range cases {
		got := cleanCMAName(c.in)
		if got != c.want {
			t.Errorf("cleanCMAName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAssignCMAs_OverrideAndParents(t *testing.T) {
	cmas := []CMA{
		// Toronto — single-province override
		{UID: "535", Name: "Toronto", ProvinceUIDs: []string{"35"}},
		// Metro Vancouver — kind override
		{UID: "933", Name: "Vancouver", ProvinceUIDs: []string{"59"}},
		// Ottawa-Gatineau — multi-province (Ontario + Quebec)
		{UID: "505", Name: "Ottawa - Gatineau", ProvinceUIDs: []string{"35", "24"}},
		// Halifax — no override, single province
		{UID: "205", Name: "Halifax", ProvinceUIDs: []string{"12"}},
		// Defensive: duplicate-province within ProvinceUIDs must dedupe.
		{UID: "950", Name: "Test Dedupe", ProvinceUIDs: []string{"35", "35"}},
	}
	// Overrides now arrive as data (regions_ca_cma_overrides.toml) rather
	// than the compiled cmaOverrides map; supply the canonical set here so
	// the assignment logic (slug/name/kind override + per-field fallback)
	// is exercised the same way the ETL run applies them.
	overrides := []CMAOverride{
		{UID: "535", Slug: "toronto-cma", Name: "Greater Toronto Area"},
		{UID: "462", Slug: "montreal-cma", Name: "Greater Montréal"},
		{UID: "933", Slug: "metro-vancouver", Name: "Metro Vancouver", Kind: "ca:regional-district"},
		{UID: "505", Slug: "ottawa-gatineau-cma", Name: "Ottawa-Gatineau"},
	}
	got := assignCMAs(cmas, overrides)

	byUID := map[string]CMAAssignment{}
	for _, a := range got {
		byUID[a.UID] = a
	}

	// Toronto: override slug + name, default kind (ca:cma), single-prov parent.
	if a := byUID["535"]; a.Slug != "toronto-cma" || a.Name != "Greater Toronto Area" || a.Kind != "ca:cma" {
		t.Errorf("Toronto = %+v", a)
	}
	if diff := cmp.Diff([]string{"on"}, byUID["535"].Parents); diff != "" {
		t.Errorf("Toronto parents (-want +got):\n%s", diff)
	}

	// Metro Vancouver: kind override applied.
	if a := byUID["933"]; a.Kind != "ca:regional-district" || a.Slug != "metro-vancouver" {
		t.Errorf("Vancouver kind/slug = %+v", a)
	}

	// Ottawa-Gatineau: multi-province → STATELESS umbrella + sorted
	// rollup_states (no parent edges; per-province routing via portions).
	if diff := cmp.Diff([]string{}, byUID["505"].Parents); diff != "" {
		t.Errorf("Ottawa-Gatineau should be stateless (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"on", "qc"}, byUID["505"].RollupStates); diff != "" {
		t.Errorf("Ottawa-Gatineau rollup_states (-want +got):\n%s", diff)
	}

	// Halifax: no override, auto-generated slug + name from source.
	if a := byUID["205"]; a.Slug != "halifax-cma" || a.Name != "Halifax" || a.Kind != "ca:cma" {
		t.Errorf("Halifax = %+v", a)
	}

	// Dedupe: two identical PRUIDs collapse to one parent.
	if diff := cmp.Diff([]string{"on"}, byUID["950"].Parents); diff != "" {
		t.Errorf("Dedupe parents (-want +got):\n%s", diff)
	}

	// Output sorted by slug ASC for deterministic TOML emission.
	for i := 1; i < len(got); i++ {
		if got[i-1].Slug > got[i].Slug {
			t.Errorf("assignCMAs not sorted: %q before %q", got[i-1].Slug, got[i].Slug)
		}
	}
}

func TestCrosswalk_ReasonPriority(t *testing.T) {
	fsas := []FSARow{
		{CFSAUID: "M5V", PRUID: "35"}, // curated city leaf → toronto (outranks its CMA)
		{CFSAUID: "M3K", PRUID: "35"}, // spatial join → toronto-cma
		{CFSAUID: "H2X", PRUID: "24"}, // city-leaf → montreal
		{CFSAUID: "V8W", PRUID: "59"}, // spatial join → victoria-cma (mid-size metro)
		{CFSAUID: "B3H", PRUID: "12"}, // no join entry → province (ns)
		{CFSAUID: "A0A", PRUID: "10"}, // no join entry → province (nl-province)
		{CFSAUID: "K1A", PRUID: "35"}, // ottawa-gatineau, Ontario side → on-portion
		{CFSAUID: "J8X", PRUID: "24"}, // ottawa-gatineau, Quebec side → qc-portion
		{CFSAUID: "X9X", PRUID: "99"}, // no join, unknown province → unknown
	}
	// cmaSlugByFSA is what ca.go builds from the max-overlap spatial join,
	// already filtered to generated CMA slugs (an FSA whose CMA wasn't
	// emitted is simply absent → it falls through to province). M5V/H2X
	// also overlap CMAs, but the city-leaf rule outranks the CMA.
	cmaSlugByFSA := map[string]string{
		"M5V": "toronto-cma",
		"M3K": "toronto-cma",
		"H2X": "montreal-cma",
		"V8W": "victoria-cma",
		"K1A": "ottawa-gatineau-cma",
		"J8X": "ottawa-gatineau-cma",
	}
	// Ottawa-Gatineau is multi-province: its FSAs route to the per-province
	// portion rather than the bare umbrella.
	portionByCMA := map[string]string{
		"ottawa-gatineau-cma:35": "ottawa-gatineau-cma-on",
		"ottawa-gatineau-cma:24": "ottawa-gatineau-cma-qc",
	}

	anchors, reasons := Crosswalk(fsas, cmaSlugByFSA, portionByCMA)
	got := map[string]PostalAnchor{}
	for _, a := range anchors {
		got[a.FSA] = a
	}

	type want struct {
		slug, reason string
	}
	expectations := map[string]want{
		"M5V": {"toronto", "city-leaf"},
		"M3K": {"toronto-cma", "cma"},
		"H2X": {"montreal", "city-leaf"},
		"V8W": {"victoria-cma", "cma"},
		"B3H": {"ns", "province"},
		"A0A": {"nl-province", "province"},
		"K1A": {"ottawa-gatineau-cma-on", "cma-portion"},
		"J8X": {"ottawa-gatineau-cma-qc", "cma-portion"},
	}
	for fsa, w := range expectations {
		a, ok := got[fsa]
		if !ok {
			t.Errorf("FSA %s missing", fsa)
			continue
		}
		if a.AnchorSlug != w.slug || a.Reason != w.reason {
			t.Errorf("FSA %s = (%s,%s), want (%s,%s)", fsa, a.AnchorSlug, a.Reason, w.slug, w.reason)
		}
	}
	if _, ok := got["X9X"]; ok {
		t.Errorf("X9X should have been dropped to unknown bucket")
	}
	if reasons["unknown"] != 1 {
		t.Errorf("unknown count = %d, want 1", reasons["unknown"])
	}
	// Anchors sorted by FSA for deterministic CSV emission.
	for i := 1; i < len(anchors); i++ {
		if anchors[i-1].FSA > anchors[i].FSA {
			t.Errorf("anchors not sorted: %q before %q", anchors[i-1].FSA, anchors[i].FSA)
		}
	}
}

func TestLatin1ToUTF8(t *testing.T) {
	// Single-byte Latin-1 0xE9 ('é' in Latin-1) round-trips to the
	// UTF-8 representation of U+00E9 (two bytes).
	got := latin1ToUTF8([]byte{0x4D, 0x6F, 0x6E, 0x74, 0x72, 0xE9, 0x61, 0x6C}) // "Montréal" Latin-1
	if got != "Montréal" {
		t.Errorf("latin1ToUTF8(Montréal) = %q, want %q", got, "Montréal")
	}
}

// TestDBFReader exercises the stdlib-only DBF parser against a tiny
// hand-built fixture: 2 character fields, 3 records (active, deleted,
// active-with-Latin-1). Verifies record skipping, Latin-1 → UTF-8
// promotion, and io.EOF termination.
func TestDBFReader(t *testing.T) {
	const (
		fieldACode = "CODE"
		fieldBName = "NAME"
		codeLen    = 3
		nameLen    = 12
	)
	recordLen := 1 + codeLen + nameLen // 1 = deletion flag

	var buf bytes.Buffer

	// File header (32 bytes).
	header := make([]byte, 32)
	header[0] = 0x03                                                // dBASE III
	binary.LittleEndian.PutUint32(header[4:8], 3)                   // 3 records
	binary.LittleEndian.PutUint16(header[8:10], uint16(32+32+32+1)) // 32 file-header + 2×32 field-descriptors + 1 terminator
	binary.LittleEndian.PutUint16(header[10:12], uint16(recordLen)) // record size
	buf.Write(header)

	// Field A descriptor.
	fa := make([]byte, 32)
	copy(fa[0:11], fieldACode)
	fa[11] = 'C'
	fa[16] = codeLen
	buf.Write(fa)

	// Field B descriptor.
	fb := make([]byte, 32)
	copy(fb[0:11], fieldBName)
	fb[11] = 'C'
	fb[16] = nameLen
	buf.Write(fb)

	// Field descriptor terminator.
	buf.WriteByte(0x0D)

	// Record 1: active. CODE="001", NAME="Toronto".
	r1 := make([]byte, recordLen)
	r1[0] = ' '
	copy(r1[1:1+codeLen], "001")
	copy(r1[1+codeLen:1+codeLen+nameLen], pad("Toronto", nameLen))
	buf.Write(r1)

	// Record 2: deleted. Reader should skip it.
	r2 := make([]byte, recordLen)
	r2[0] = '*'
	copy(r2[1:1+codeLen], "XXX")
	copy(r2[1+codeLen:1+codeLen+nameLen], pad("Ignored", nameLen))
	buf.Write(r2)

	// Record 3: active, Latin-1 'é' (0xE9) in NAME.
	r3 := make([]byte, recordLen)
	r3[0] = ' '
	copy(r3[1:1+codeLen], "002")
	r3Name := []byte{'M', 'o', 'n', 't', 'r', 0xE9, 'a', 'l', ' ', ' ', ' ', ' '} // padded to nameLen
	copy(r3[1+codeLen:1+codeLen+nameLen], r3Name)
	buf.Write(r3)

	dbf, err := newDBFReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("newDBFReader: %v", err)
	}
	if len(dbf.fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(dbf.fields))
	}
	if dbf.fields[0].Name != fieldACode || dbf.fields[1].Name != fieldBName {
		t.Errorf("fields = %+v", dbf.fields)
	}

	// Record 1.
	row, err := dbf.next()
	if err != nil {
		t.Fatalf("next #1: %v", err)
	}
	if row[fieldACode] != "001" || row[fieldBName] != "Toronto" {
		t.Errorf("record 1 = %+v", row)
	}

	// Record 2 is deleted; next() should skip to record 3.
	row, err = dbf.next()
	if err != nil {
		t.Fatalf("next #2 (post-skip): %v", err)
	}
	if row[fieldACode] != "002" {
		t.Errorf("expected to skip deleted row and land on 002, got %+v", row)
	}
	if row[fieldBName] != "Montréal" {
		t.Errorf("Latin-1 decode: %q, want %q", row[fieldBName], "Montréal")
	}

	// Exhausted.
	if _, err := dbf.next(); err != io.EOF {
		t.Errorf("expected io.EOF after exhausting records, got %v", err)
	}
}

func pad(s string, n int) []byte {
	out := make([]byte, n)
	copy(out, s)
	for i := len(s); i < n; i++ {
		out[i] = ' '
	}
	return out
}
