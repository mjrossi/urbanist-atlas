package ca

import (
	"fmt"
	"sort"
	"strings"

	polyclip "github.com/ctessum/polyclip-go"
	shp "github.com/jonas-p/go-shp"
)

// The FSA → CMA spatial join assigns each FSA to the CMA it overlaps most
// by area. Both StatsCan boundary files are EPSG:3347 (Lambert, meters),
// so planar areas compare directly with no reprojection. Why max-overlap
// and not point-in-polygon: docs/architecture/etl-and-stores.md.

// cmaGeometry is one CMA's boundary, dissolved across every type-'B'
// record sharing its CMAUID (Ottawa-Gatineau has one per province).
type cmaGeometry struct {
	uid      string
	parts    []polyclip.Polygon   // one polygon per source shapefile record
	partBBox []polyclip.Rectangle // bounding box of parts[i], precomputed at load
	bbox     polyclip.Rectangle   // union of partBBox (whole-CMA bounding box)
}

// minOverlapFraction is the share of an FSA's area that must fall in a
// CMA for the FSA to anchor there instead of at its province. StatsCan
// digitizes FSA and CMA boundaries separately, leaving sub-square-meter
// slivers along shared edges. In the 2021 vintage the slivers sit at or
// below 1e-6 of FSA area and real overlaps start at 1e-3, so 0.1% strips
// the noise without imposing a majority rule.
const minOverlapFraction = 0.001

// spatialJoinFSAToCMA returns CFSAUID → raw CMAUID for every FSA whose
// largest CMA overlap clears minOverlapFraction. Other FSAs are omitted,
// so they fall through to their province.
func spatialJoinFSAToCMA(fsaZipPath, cmaZipPath string) (map[string]string, error) {
	cmas, err := loadCMAGeometry(cmaZipPath)
	if err != nil {
		return nil, err
	}
	return assignFSAsToCMAs(fsaZipPath, cmas)
}

// loadCMAGeometry reads the type-'B' CMA records, dissolved by CMAUID and
// sorted by UID so the max-overlap tie-break is deterministic.
func loadCMAGeometry(zipPath string) ([]cmaGeometry, error) {
	zr, err := openShapeFromZip(zipPath)
	if err != nil {
		return nil, fmt.Errorf("cma geometry: %w", err)
	}
	defer zr.Close()

	iUID := fieldIndexByName(zr.Fields(), "CMAUID")
	iType := fieldIndexByName(zr.Fields(), "CMATYPE")
	if iUID < 0 || iType < 0 {
		return nil, fmt.Errorf("cma geometry: missing CMAUID/CMATYPE field in %s", zipPath)
	}

	byUID := map[string]*cmaGeometry{}
	var order []string
	for zr.Next() {
		_, shape := zr.Shape()
		if strings.Trim(zr.Attribute(iType), " \x00") != "B" {
			continue
		}
		poly, ok := shapePolygon(shape)
		if !ok {
			continue
		}
		uid := strings.Trim(zr.Attribute(iUID), " \x00")
		if uid == "" {
			continue
		}
		g := byUID[uid]
		if g == nil {
			g = &cmaGeometry{uid: uid}
			byUID[uid] = g
			order = append(order, uid)
		}
		g.parts = append(g.parts, poly)
	}
	if err := zr.Err(); err != nil {
		return nil, fmt.Errorf("cma geometry: read %s: %w", zipPath, err)
	}

	out := make([]cmaGeometry, 0, len(order))
	for _, uid := range order {
		g := byUID[uid]
		g.partBBox = make([]polyclip.Rectangle, len(g.parts))
		for i, part := range g.parts {
			g.partBBox[i] = part.BoundingBox()
		}
		g.bbox = unionBBoxes(g.partBBox)
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].uid < out[j].uid })
	return out, nil
}

// assignFSAsToCMAs assigns each FSA to its max-overlap CMA, clipping
// only the (FSA, CMA) pairs whose bounding boxes intersect.
func assignFSAsToCMAs(zipPath string, cmas []cmaGeometry) (map[string]string, error) {
	zr, err := openShapeFromZip(zipPath)
	if err != nil {
		return nil, fmt.Errorf("fsa geometry: %w", err)
	}
	defer zr.Close()

	iFSA := fieldIndexByName(zr.Fields(), "CFSAUID")
	if iFSA < 0 {
		return nil, fmt.Errorf("fsa geometry: missing CFSAUID field in %s", zipPath)
	}

	out := map[string]string{}
	for zr.Next() {
		_, shape := zr.Shape()
		fsa := strings.Trim(zr.Attribute(iFSA), " \x00")
		if fsa == "" {
			continue
		}
		poly, ok := shapePolygon(shape)
		if !ok {
			continue
		}
		fbb := poly.BoundingBox()
		// polyclip's sweep-line loses precision at EPSG:3347's ~9e6 m
		// coordinates and returns near-empty intersections for polygons that
		// genuinely overlap, so translate to the FSA's local origin first.
		// fsaArea is the cheap gross area: it is only the floor's denominator,
		// and exact even-odd area is far too slow on archipelago FSAs.
		ox, oy := fbb.Min.X, fbb.Min.Y
		local := translate(poly, ox, oy)
		fsaArea := polygonGrossArea(local)

		bestUID, bestArea := "", 0.0
		for i := range cmas {
			c := &cmas[i]
			if !bboxesOverlap(fbb, c.bbox) {
				continue
			}
			var area float64
			for pi, part := range c.parts {
				if !bboxesOverlap(fbb, c.partBBox[pi]) {
					continue
				}
				area += polygonArea(local.Construct(polyclip.INTERSECTION, translate(part, ox, oy)))
			}
			// cmas is UID-sorted and only a strictly larger area wins, so ties
			// go to the smallest UID.
			if area > bestArea {
				bestArea, bestUID = area, c.uid
			}
		}
		if bestUID != "" && fsaArea > 0 && bestArea >= minOverlapFraction*fsaArea {
			out[fsa] = bestUID
		}
	}
	if err := zr.Err(); err != nil {
		return nil, fmt.Errorf("fsa geometry: read %s: %w", zipPath, err)
	}
	return out, nil
}

// openShapeFromZip opens the single shapefile inside a StatsCan boundary
// zip, at the root (CMA) or in a subdirectory (FSA).
func openShapeFromZip(zipPath string) (*shp.ZipReader, error) {
	names, err := shp.ShapesInZip(zipPath)
	if err != nil {
		return nil, fmt.Errorf("list shapes in %s: %w", zipPath, err)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no shapefile in %s", zipPath)
	}
	zr, err := shp.OpenShapeFromZip(zipPath, names[0])
	if err != nil {
		return nil, fmt.Errorf("open shape %s in %s: %w", names[0], zipPath, err)
	}
	return zr, nil
}

// fieldIndexByName resolves a DBF column name to its index for
// ZipReader.Attribute. go-shp pads field names with NULs to 11 bytes.
func fieldIndexByName(fields []shp.Field, name string) int {
	for i, f := range fields {
		if strings.EqualFold(strings.TrimRight(f.String(), "\x00"), name) {
			return i
		}
	}
	return -1
}

// shapePolygon converts a go-shp Polygon into a polyclip Polygon, one
// Contour per ring. Holes go in as plain contours: polyclip reads a
// polygon by the even-odd rule, so nesting, not winding, makes a hole.
func shapePolygon(s shp.Shape) (polyclip.Polygon, bool) {
	p, ok := s.(*shp.Polygon)
	if !ok {
		return nil, false
	}
	bounds := make([]int32, len(p.Parts), len(p.Parts)+1)
	copy(bounds, p.Parts)
	bounds = append(bounds, int32(len(p.Points)))

	var poly polyclip.Polygon
	for i := 0; i+1 < len(bounds); i++ {
		ring := p.Points[bounds[i]:bounds[i+1]]
		if len(ring) < 3 {
			continue
		}
		c := make(polyclip.Contour, 0, len(ring))
		for _, pt := range ring {
			c = append(c, polyclip.Point{X: pt.X, Y: pt.Y})
		}
		poly.Add(c)
	}
	if len(poly) == 0 {
		return nil, false
	}
	return poly, true
}
