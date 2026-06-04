package ca

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	shp "github.com/jonas-p/go-shp"
)

// dbfFieldDef is one character ('C') column for a fixture shapefile's
// attribute table.
type dbfFieldDef struct {
	Name string
	Len  int
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
