package ca

import (
	"fmt"
	"sort"
	"strings"

	polyclip "github.com/ctessum/polyclip-go"
	shp "github.com/jonas-p/go-shp"
)

// spatial.go implements the FSA → CMA assignment by a max-overlap spatial
// join of the StatsCan boundary-file polygons. The DBF-only parse path
// (cma.go / fsa.go) reads just the attribute tables; this reads the .shp
// geometry those files ignore and assigns each Forward Sortation Area to
// the Census Metropolitan Area it overlaps most by area.
//
// Both StatsCan boundary files are published in EPSG:3347 (NAD83
// Statistics Canada Lambert, metres), so planar intersection area is
// directly comparable and no reprojection is needed. The .prj inside each
// zip is identical (same product series); we trust that rather than
// parsing the WKT.
//
// This replaces the coarse hand-coded FSA-prefix → CMA table that covered
// only the seven biggest metros: a prefix rule can't separate adjacent
// CMAs sharing a prefix (Victoria and Nanaimo are both V9), whereas the
// spatial join resolves all ~41 CMAs deterministically.

// cmaGeometry is the dissolved boundary of one Census Metropolitan Area —
// every type-'B' shapefile record sharing a CMAUID (a multi-province CMA
// such as Ottawa-Gatineau appears as one record per province) — plus its
// bounding box, used as the clip target in the spatial join.
type cmaGeometry struct {
	uid   string
	parts []polyclip.Polygon // one polygon per source shapefile record
	bbox  polyclip.Rectangle
}

// SpatialJoinFSAToCMA assigns each FSA to the CMA it overlaps most by
// area. It returns CFSAUID → CMAUID for every FSA that overlaps at least
// one type-'B' CMA; FSAs that overlap none are omitted, so the crosswalk
// falls them through to their province. The returned CMAUIDs are the raw
// StatsCan codes (e.g. "535"); the caller resolves them to region slugs
// via the CMA assignments.
func SpatialJoinFSAToCMA(fsaZipPath, cmaZipPath string) (map[string]string, error) {
	cmas, err := loadCMAGeometry(cmaZipPath)
	if err != nil {
		return nil, err
	}
	return assignFSAsToCMAs(fsaZipPath, cmas)
}

// loadCMAGeometry reads the CMA boundary shapefile, keeps only type-'B'
// records (true CMAs, matching ParseCMAs), and dissolves them by CMAUID
// into one cmaGeometry per metro. The result is sorted by UID so the
// max-overlap tie-break (smallest UID wins) is deterministic.
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
		if strings.TrimSpace(zr.Attribute(iType)) != "B" {
			continue
		}
		poly, ok := shapePolygon(shape)
		if !ok {
			continue
		}
		uid := strings.TrimSpace(zr.Attribute(iUID))
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
		g.bbox = polygonsBBox(g.parts)
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].uid < out[j].uid })
	return out, nil
}

// assignFSAsToCMAs streams the FSA boundary shapefile and assigns each FSA
// to the max-overlap CMA. A bounding-box pre-filter skips the polygon
// clipping for the vast majority of (FSA, CMA) pairs that can't overlap;
// only the handful whose boxes intersect are clipped for exact area.
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
		fsa := strings.TrimSpace(zr.Attribute(iFSA))
		if fsa == "" {
			continue
		}
		poly, ok := shapePolygon(shape)
		if !ok {
			continue
		}
		fbb := poly.BoundingBox()

		bestUID, bestArea := "", 0.0
		for i := range cmas {
			c := &cmas[i]
			if !bboxesOverlap(fbb, c.bbox) {
				continue
			}
			var area float64
			for _, part := range c.parts {
				if !bboxesOverlap(fbb, part.BoundingBox()) {
					continue
				}
				area += polygonArea(poly.Construct(polyclip.INTERSECTION, part))
			}
			// cmas is sorted by UID ascending and we only replace on a
			// strictly larger area, so the smallest UID wins any tie
			// deterministically (real-data ties are effectively impossible).
			if area > bestArea {
				bestArea, bestUID = area, c.uid
			}
		}
		if bestUID != "" {
			out[fsa] = bestUID
		}
	}
	if err := zr.Err(); err != nil {
		return nil, fmt.Errorf("fsa geometry: read %s: %w", zipPath, err)
	}
	return out, nil
}

// openShapeFromZip locates the single shapefile inside a StatsCan boundary
// zip and opens it with attribute access. The .shp may sit at the zip root
// (the CMA file) or under a subdirectory (the FSA file); ShapesInZip finds
// it either way.
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

// shapePolygon converts a go-shp Polygon (rings flattened into Points and
// delimited by Parts) into a polyclip Polygon (one Contour per ring).
// Outer rings and holes are both passed as contours; polyclip's even-odd
// handling yields the correct net intersection area downstream. Returns
// false for non-polygon or empty geometry.
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

// polygonArea returns the absolute net area of a polyclip Polygon via the
// shoelace formula summed over its rings: outer rings contribute positive
// signed area and holes negative, so the magnitude of the sum is the area
// with holes removed.
func polygonArea(p polyclip.Polygon) float64 {
	var signed float64
	for _, c := range p {
		n := len(c)
		if n < 3 {
			continue
		}
		for i := range n {
			j := i + 1
			if j == n {
				j = 0
			}
			signed += c[i].X*c[j].Y - c[j].X*c[i].Y
		}
	}
	if signed < 0 {
		signed = -signed
	}
	return signed / 2
}

// polygonsBBox returns the bounding box enclosing all of a CMA's polygon
// parts.
func polygonsBBox(polys []polyclip.Polygon) polyclip.Rectangle {
	var box polyclip.Rectangle
	first := true
	for _, p := range polys {
		bb := p.BoundingBox()
		if first {
			box = bb
			first = false
			continue
		}
		if bb.Min.X < box.Min.X {
			box.Min.X = bb.Min.X
		}
		if bb.Min.Y < box.Min.Y {
			box.Min.Y = bb.Min.Y
		}
		if bb.Max.X > box.Max.X {
			box.Max.X = bb.Max.X
		}
		if bb.Max.Y > box.Max.Y {
			box.Max.Y = bb.Max.Y
		}
	}
	return box
}

// bboxesOverlap reports whether two axis-aligned boxes intersect (touching
// edges count as overlap — the subsequent clip resolves the true area).
func bboxesOverlap(a, b polyclip.Rectangle) bool {
	return a.Min.X <= b.Max.X && a.Max.X >= b.Min.X &&
		a.Min.Y <= b.Max.Y && a.Max.Y >= b.Min.Y
}
