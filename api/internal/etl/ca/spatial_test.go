package ca

import (
	"archive/zip"
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"

	polyclip "github.com/ctessum/polyclip-go"
	shp "github.com/jonas-p/go-shp"
)

// boxContour builds a polyclip.Contour for an axis-aligned box. cw selects
// clockwise vs counter-clockwise vertex order so tests can prove polygonArea
// ignores winding (polyclip's boolean output gives holes no guaranteed
// winding).
func boxContour(minX, minY, maxX, maxY float64, cw bool) polyclip.Contour {
	c := polyclip.Contour{
		{X: minX, Y: minY},
		{X: minX, Y: maxY},
		{X: maxX, Y: maxY},
		{X: maxX, Y: minY},
	}
	if cw {
		return c
	}
	rev := make(polyclip.Contour, len(c))
	for i, p := range c {
		rev[len(c)-1-i] = p
	}
	return rev
}

// holedShape builds a *shp.Polygon from one or more rings (ring 0 is the
// outer boundary; any further rings are holes), the production input shape
// that shapePolygon consumes.
func holedShape(rings ...[]shp.Point) shp.Shape {
	poly := shp.Polygon(*shp.NewPolyLine(rings))
	return &poly
}

// square returns a closed clockwise ring for the axis-aligned box
// [minX,minY]–[maxX,maxY], used to build fixture polygon geometry. The
// spatial join only cares about overlap area, so simple boxes suffice.
func square(minX, minY, maxX, maxY float64) []shp.Point {
	return []shp.Point{
		{X: minX, Y: minY},
		{X: minX, Y: maxY},
		{X: maxX, Y: maxY},
		{X: maxX, Y: minY},
		{X: minX, Y: minY},
	}
}

// dbfFieldDef is one character ('C') column for a fixture shapefile's
// attribute table.
type dbfFieldDef struct {
	Name string
	Len  int
}

// writeShapefileZip writes a StatsCan-style boundary zip at zipPath
// containing a polygon shapefile (.shp/.shx/.dbf) built with go-shp's
// Writer. fields/rows mirror buildDBF (all character columns); rings[i] is
// the outer ring for rows[i]. The single coherent shapefile is consumed by
// both the internal dbf.go parser (attribute table, via openDBFFromZip)
// and the go-shp spatial reader (geometry + attributes, via spatial.go).
func writeShapefileZip(t *testing.T, zipPath, base string, fields []dbfFieldDef, rows [][]string, rings [][]shp.Point) {
	t.Helper()
	if len(rows) != len(rings) {
		t.Fatalf("writeShapefileZip: %d rows but %d rings", len(rows), len(rings))
	}

	dir := t.TempDir()
	shpPath := filepath.Join(dir, base+".shp")
	w, err := shp.Create(shpPath, shp.POLYGON)
	if err != nil {
		t.Fatalf("shp.Create: %v", err)
	}
	shpFields := make([]shp.Field, len(fields))
	for i, f := range fields {
		shpFields[i] = shp.StringField(f.Name, uint8(f.Len))
	}
	if err := w.SetFields(shpFields); err != nil {
		t.Fatalf("SetFields: %v", err)
	}
	for ri, ring := range rings {
		poly := shp.Polygon(*shp.NewPolyLine([][]shp.Point{ring}))
		row := w.Write(&poly)
		for fi := range fields {
			var v string
			if fi < len(rows[ri]) {
				v = rows[ri][fi]
			}
			if err := w.WriteAttribute(int(row), fi, v); err != nil {
				t.Fatalf("WriteAttribute: %v", err)
			}
		}
	}
	w.Close()

	// Bundle the three shapefile members at the zip root (the CMA boundary
	// zip's layout; the readers locate members by extension regardless).
	// go-shp's Writer (v0.1.1) emits the DBF on disk as "<base>dbf" with
	// no dot, so read from that path but name the zip member "<base>.dbf"
	// — what both go-shp's reader (prefix+".dbf") and dbf.go expect.
	members := []struct{ zipName, diskName string }{
		{base + ".shp", base + ".shp"},
		{base + ".shx", base + ".shx"},
		{base + ".dbf", base + "dbf"},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, m := range members {
		data, err := os.ReadFile(filepath.Join(dir, m.diskName))
		if err != nil {
			t.Fatalf("read %s: %v", m.diskName, err)
		}
		zm, err := zw.Create(m.zipName)
		if err != nil {
			t.Fatalf("zip create %s: %v", m.zipName, err)
		}
		if _, err := zm.Write(data); err != nil {
			t.Fatalf("zip write %s: %v", m.zipName, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip %s: %v", zipPath, err)
	}
}

// TestPolygonArea_SubtractsHoles pins the core area math: a polygon with a
// hole must measure outer-minus-hole, and the result must not depend on the
// winding of either contour (polyclip's connector emits result contours in
// arbitrary winding). Outer 100×100 = 10000, hole 40×40 = 1600 → net 8400.
func TestPolygonArea_SubtractsHoles(t *testing.T) {
	const want = 8400.0
	for _, outerCW := range []bool{true, false} {
		for _, holeCW := range []bool{true, false} {
			poly := polyclip.Polygon{
				boxContour(0, 0, 100, 100, outerCW),
				boxContour(30, 30, 70, 70, holeCW),
			}
			if got := polygonArea(poly); math.Abs(got-want) > 1e-9 {
				t.Errorf("polygonArea(outerCW=%v, holeCW=%v) = %v, want %v",
					outerCW, holeCW, got, want)
			}
		}
	}
}

// TestPolygonArea_HoleThroughConstruct exercises the real production path:
// an FSA polygon with a hole, intersected with a CMA that fully contains it.
// shapePolygon → Construct(INTERSECTION) → polygonArea must yield the holed
// FSA's true area (outer 10000 − hole 1600 = 8400), not 11600.
func TestPolygonArea_HoleThroughConstruct(t *testing.T) {
	fsa := holedShape(square(0, 0, 100, 100), square(30, 30, 70, 70)) // net 8400
	cma := holedShape(square(-50, -50, 150, 150))                     // solid container

	fp, ok := shapePolygon(fsa)
	if !ok {
		t.Fatal("shapePolygon(fsa) returned !ok")
	}
	cp, ok := shapePolygon(cma)
	if !ok {
		t.Fatal("shapePolygon(cma) returned !ok")
	}

	got := polygonArea(fp.Construct(polyclip.INTERSECTION, cp))
	if math.Abs(got-8400.0) > 1e-9 {
		t.Errorf("intersection area = %v, want 8400 (hole subtracted)", got)
	}
}

// TestPolygonArea_HoleVertexOnContourBoundary pins the nestingDepth fix: a
// hole whose first vertex lies exactly on the outer ring's boundary must still
// be subtracted. With the old raw-vertex representative, Contour.Contains
// classified a point on the outer's top/right edge as outside, so the hole was
// added (10800 instead of 9200). interiorPoint sidesteps the boundary.
func TestPolygonArea_HoleVertexOnContourBoundary(t *testing.T) {
	outer := polyclip.Contour{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 100, Y: 100}, {X: 0, Y: 100}}
	// Each hole has area 800; net = 10000 - 800 = 9200. The hole's FIRST
	// vertex sits on the outer ring — right edge (x=100) and top edge (y=100)
	// respectively — the exact configuration that flipped the parity before.
	cases := []struct {
		name string
		hole polyclip.Contour
	}{
		{"right-edge", polyclip.Contour{{X: 100, Y: 40}, {X: 60, Y: 40}, {X: 60, Y: 60}, {X: 100, Y: 60}}},
		{"top-edge", polyclip.Contour{{X: 40, Y: 100}, {X: 40, Y: 60}, {X: 60, Y: 60}, {X: 60, Y: 100}}},
	}
	for _, tc := range cases {
		if got := polygonArea(polyclip.Polygon{outer, tc.hole}); math.Abs(got-9200.0) > 1e-9 {
			t.Errorf("polygonArea(hole first-vertex on %s) = %v, want 9200 (hole subtracted)", tc.name, got)
		}
	}
}

// TestSpatialJoin_StraddleAssignsLargerOverlap covers the headline behavior
// the slice exists for: an FSA spanning two CMAs is assigned to the one it
// overlaps most by area — even when that CMA has the higher UID, proving
// area (not UID order) decides.
func TestSpatialJoin_StraddleAssignsLargerOverlap(t *testing.T) {
	dir := t.TempDir()
	cmaFields := []dbfFieldDef{{"CMAUID", 3}, {"CMATYPE", 1}}
	cmaRows := [][]string{{"100", "B"}, {"200", "B"}}
	cmaRings := [][]shp.Point{
		square(0, 0, 100, 100),   // CMA 100 (west)
		square(100, 0, 200, 100), // CMA 200 (east)
	}
	writeShapefileZip(t, filepath.Join(dir, "cma.zip"), "cma", cmaFields, cmaRows, cmaRings)

	fsaFields := []dbfFieldDef{{"CFSAUID", 3}}
	fsaRows := [][]string{{"X1X"}}
	// x∈[60,160]: 40 wide inside CMA 100, 60 wide inside CMA 200 → CMA 200
	// wins on area despite the higher UID.
	fsaRings := [][]shp.Point{square(60, 0, 160, 100)}
	writeShapefileZip(t, filepath.Join(dir, "fsa.zip"), "fsa", fsaFields, fsaRows, fsaRings)

	got, err := SpatialJoinFSAToCMA(filepath.Join(dir, "fsa.zip"), filepath.Join(dir, "cma.zip"))
	if err != nil {
		t.Fatalf("SpatialJoinFSAToCMA: %v", err)
	}
	if got["X1X"] != "200" {
		t.Errorf("X1X assigned to %q, want 200 (larger overlap)", got["X1X"])
	}
}

// TestSpatialJoin_LargeOffsetStraddle runs the full join end-to-end at the
// ~9e6 m EPSG:3347 offset of real StatsCan coordinates, where polyclip's
// sweep-line loses precision unless the geometry is translated to a local
// origin first (which assignFSAsToCMAs does). Without that translation the
// intersection areas collapse toward zero and the max-overlap winner is
// arbitrary — so this guards the precision fix through SpatialJoinFSAToCMA,
// not just the unit-level TestPolygonArea_AccurateAtLargeOffset. Same geometry
// as the straddle test shifted by (9e6, 2e6): 40 wide in CMA 100, 60 wide in
// CMA 200 → CMA 200 wins.
func TestSpatialJoin_LargeOffsetStraddle(t *testing.T) {
	const ox, oy = 9_000_000.0, 2_000_000.0
	off := func(ring []shp.Point) []shp.Point {
		out := make([]shp.Point, len(ring))
		for i, p := range ring {
			out[i] = shp.Point{X: p.X + ox, Y: p.Y + oy}
		}
		return out
	}
	dir := t.TempDir()
	cmaFields := []dbfFieldDef{{"CMAUID", 3}, {"CMATYPE", 1}}
	cmaRows := [][]string{{"100", "B"}, {"200", "B"}}
	cmaRings := [][]shp.Point{
		off(square(0, 0, 100, 100)),   // CMA 100 (west)
		off(square(100, 0, 200, 100)), // CMA 200 (east)
	}
	writeShapefileZip(t, filepath.Join(dir, "cma.zip"), "cma", cmaFields, cmaRows, cmaRings)

	fsaFields := []dbfFieldDef{{"CFSAUID", 3}}
	fsaRows := [][]string{{"X1X"}}
	fsaRings := [][]shp.Point{off(square(60, 0, 160, 100))}
	writeShapefileZip(t, filepath.Join(dir, "fsa.zip"), "fsa", fsaFields, fsaRows, fsaRings)

	got, err := SpatialJoinFSAToCMA(filepath.Join(dir, "fsa.zip"), filepath.Join(dir, "cma.zip"))
	if err != nil {
		t.Fatalf("SpatialJoinFSAToCMA: %v", err)
	}
	if got["X1X"] != "200" {
		t.Errorf("X1X assigned to %q at 9e6 offset, want 200 (larger overlap; precision fix)", got["X1X"])
	}
}

// TestSpatialJoin_EqualOverlapTieBreaksToSmallerUID covers the deterministic
// tie-break: an FSA overlapping two CMAs by exactly equal area resolves to
// the smaller UID (cmas are UID-sorted and only a strictly larger area
// displaces the incumbent).
func TestSpatialJoin_EqualOverlapTieBreaksToSmallerUID(t *testing.T) {
	dir := t.TempDir()
	cmaFields := []dbfFieldDef{{"CMAUID", 3}, {"CMATYPE", 1}}
	cmaRows := [][]string{{"100", "B"}, {"200", "B"}}
	cmaRings := [][]shp.Point{
		square(0, 0, 100, 100),
		square(100, 0, 200, 100),
	}
	writeShapefileZip(t, filepath.Join(dir, "cma.zip"), "cma", cmaFields, cmaRows, cmaRings)

	fsaFields := []dbfFieldDef{{"CFSAUID", 3}}
	fsaRows := [][]string{{"Y1Y"}}
	// x∈[50,150]: 50 wide in each CMA → exact area tie (axis-aligned box
	// clips give integer-exact areas).
	fsaRings := [][]shp.Point{square(50, 0, 150, 100)}
	writeShapefileZip(t, filepath.Join(dir, "fsa.zip"), "fsa", fsaFields, fsaRows, fsaRings)

	got, err := SpatialJoinFSAToCMA(filepath.Join(dir, "fsa.zip"), filepath.Join(dir, "cma.zip"))
	if err != nil {
		t.Fatalf("SpatialJoinFSAToCMA: %v", err)
	}
	if got["Y1Y"] != "100" {
		t.Errorf("Y1Y assigned to %q, want 100 (smaller UID on tie)", got["Y1Y"])
	}
}

// TestPolygonArea_AccurateAtLargeOffset guards the numerical hygiene the real
// data needs: EPSG:3347 coordinates sit ~9e6 m from the false origin, where
// the raw shoelace products reach ~1e13 and catastrophic float cancellation
// corrupts small areas. polygonArea must translate to a local origin first.
// A 1000 × 0.01 sliver has area exactly 10 regardless of where it sits.
func TestPolygonArea_AccurateAtLargeOffset(t *testing.T) {
	const ox, oy = 9_000_000.0, 2_000_000.0
	poly := polyclip.Polygon{polyclip.Contour{
		{X: ox, Y: oy},
		{X: ox + 1000, Y: oy},
		{X: ox + 1000, Y: oy + 0.01},
		{X: ox, Y: oy + 0.01},
	}}
	if got := polygonArea(poly); math.Abs(got-10.0) > 1e-6 {
		t.Errorf("polygonArea at 9e6 offset = %v, want 10 (±1e-6)", got)
	}
}

// TestPolygonGrossArea_CountsHolesPositiveAndIsOffsetStable pins the cheap
// denominator used by the overlap floor: it counts holes as positive (an
// intentional over-estimate, outer 10000 + hole 1600 = 11600) and stays
// accurate at the ~9e6 m offset of real EPSG:3347 coordinates.
func TestPolygonGrossArea_CountsHolesPositiveAndIsOffsetStable(t *testing.T) {
	const off = 9_000_000.0
	poly := polyclip.Polygon{
		boxContour(off, off, off+100, off+100, true),
		boxContour(off+30, off+30, off+70, off+70, true),
	}
	if got := polygonGrossArea(poly); math.Abs(got-11600.0) > 1e-6 {
		t.Errorf("polygonGrossArea = %v, want 11600 (holes positive, offset-stable)", got)
	}
}

// TestSpatialJoin_NoiseSliverFallsThrough is the decisive correctness case:
// separately digitized FSA and CMA boundaries leave sub-square-meter slivers
// along shared edges, so an FSA merely running alongside a CMA boundary must
// fall through to its province, not anchor to that metro on noise.
func TestSpatialJoin_NoiseSliverFallsThrough(t *testing.T) {
	dir := t.TempDir()
	cmaFields := []dbfFieldDef{{"CMAUID", 3}, {"CMATYPE", 1}}
	cmaRows := [][]string{{"100", "B"}}
	cmaRings := [][]shp.Point{square(99.99, 0, 200, 100)}
	writeShapefileZip(t, filepath.Join(dir, "cma.zip"), "cma", cmaFields, cmaRows, cmaRings)

	fsaFields := []dbfFieldDef{{"CFSAUID", 3}}
	fsaRows := [][]string{{"Z9Z"}}
	// FSA area 10000; overlap = 0.01 wide × 100 = 1 → 0.01% of the FSA, an
	// order of magnitude below the 0.1% floor.
	fsaRings := [][]shp.Point{square(0, 0, 100, 100)}
	writeShapefileZip(t, filepath.Join(dir, "fsa.zip"), "fsa", fsaFields, fsaRows, fsaRings)

	got, err := SpatialJoinFSAToCMA(filepath.Join(dir, "fsa.zip"), filepath.Join(dir, "cma.zip"))
	if err != nil {
		t.Fatalf("SpatialJoinFSAToCMA: %v", err)
	}
	if uid, ok := got["Z9Z"]; ok {
		t.Errorf("Z9Z anchored to CMA %q on a 0.1%% sliver; want province fallback (absent)", uid)
	}
}

// TestSpatialJoin_SubstantialOverlapAnchors guards that the overlap floor is
// not so aggressive it drops genuine membership: an FSA wholly inside a CMA
// (100% overlap) must still anchor.
func TestSpatialJoin_SubstantialOverlapAnchors(t *testing.T) {
	dir := t.TempDir()
	cmaFields := []dbfFieldDef{{"CMAUID", 3}, {"CMATYPE", 1}}
	cmaRows := [][]string{{"100", "B"}}
	cmaRings := [][]shp.Point{square(-50, -50, 150, 150)} // fully contains the FSA
	writeShapefileZip(t, filepath.Join(dir, "cma.zip"), "cma", cmaFields, cmaRows, cmaRings)

	fsaFields := []dbfFieldDef{{"CFSAUID", 3}}
	fsaRows := [][]string{{"A1A"}}
	fsaRings := [][]shp.Point{square(0, 0, 100, 100)}
	writeShapefileZip(t, filepath.Join(dir, "fsa.zip"), "fsa", fsaFields, fsaRows, fsaRings)

	got, err := SpatialJoinFSAToCMA(filepath.Join(dir, "fsa.zip"), filepath.Join(dir, "cma.zip"))
	if err != nil {
		t.Fatalf("SpatialJoinFSAToCMA: %v", err)
	}
	if got["A1A"] != "100" {
		t.Errorf("A1A = %q, want 100 (fully contained)", got["A1A"])
	}
}
