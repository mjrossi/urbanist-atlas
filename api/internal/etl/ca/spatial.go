package ca

import (
	"fmt"
	"math"
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
//
// Why max-overlap area, not a simpler point-in-polygon test: a
// representative-interior-point lookup (which CMA contains the FSA's
// interior point) was considered and rejected. It would drop the polygon
// clipping entirely — and with it the large-coordinate translation, the
// even-odd area math, and the noise floor below — for roughly a third
// less code. Area-overlap is the deliberate choice because it gives the
// principled answer for the FSAs that actually motivate this code: an FSA
// straddling two adjacent metros anchors to the one it sits in *most*, and
// a multi-part coastal FSA (BC's V8/V9 archipelagos) whose interior point
// could land on an uninhabited islet is assigned by total land overlap,
// not by where one point happens to fall. The two approaches agree on the
// ~1,500 FSAs that sit cleanly inside or outside a CMA and differ only on
// the ~120 partial-overlap FSAs, where "most of the FSA" is the defensible
// rule. A future maintainer tempted to simplify to point-in-polygon should
// weigh that trade-off, not just the line count.

// cmaGeometry is the dissolved boundary of one Census Metropolitan Area —
// every type-'B' shapefile record sharing a CMAUID (a multi-province CMA
// such as Ottawa-Gatineau appears as one record per province) — plus its
// bounding box, used as the clip target in the spatial join.
type cmaGeometry struct {
	uid      string
	parts    []polyclip.Polygon   // one polygon per source shapefile record
	partBBox []polyclip.Rectangle // bounding box of parts[i], precomputed at load
	bbox     polyclip.Rectangle   // union of partBBox (whole-CMA bounding box)
}

// minOverlapFraction is the smallest share of an FSA's own area that must
// fall inside a CMA for the FSA to anchor to that CMA rather than its
// province. StatsCan digitizes the FSA and CMA boundaries separately, so
// they don't perfectly coincide: a rural FSA running alongside a CMA
// boundary picks up a sub-square-metre line-work sliver. Without this floor
// that noise wins the max-overlap and wrongly anchors the FSA to a metro it
// isn't in. The floor is relative to FSA area so it is size-invariant — a
// small urban FSA wholly inside a CMA (~100%) still clears it, while a
// grazing sliver never does.
//
// Calibrated from the measured overlap distribution over the 2021 vintage:
// the noise slivers all fall at or below 1e-6 (1e-4 %) of FSA area, then
// there is a completely empty gap up to 1e-3 (0.1 %) before genuine partial
// overlaps begin — so 0.1 % sits squarely in that gap, stripping every
// noise sliver while preserving every FSA with real geometric overlap. This
// only removes noise; it deliberately does not impose a "majority in the
// metro" rule, keeping the join's max-overlap semantics intact.
const minOverlapFraction = 0.001

// SpatialJoinFSAToCMA assigns each FSA to the CMA it overlaps most by
// area. It returns CFSAUID → CMAUID for every FSA whose largest CMA overlap
// clears minOverlapFraction of the FSA's area; FSAs that overlap no CMA, or
// only by a sub-threshold line-work sliver, are omitted so the crosswalk
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
		fsa := strings.Trim(zr.Attribute(iFSA), " \x00")
		if fsa == "" {
			continue
		}
		poly, ok := shapePolygon(shape)
		if !ok {
			continue
		}
		fbb := poly.BoundingBox()
		// polyclip's sweep-line loses precision at native EPSG:3347
		// coordinates (~9e6 m): the cross-products reach ~1e14, events
		// mis-order, and Construct returns near-empty intersections for
		// polygons that genuinely overlap (e.g. an FSA wholly inside its
		// CMA computing as 0). The failure is data-dependent, so it
		// silently corrupts only some anchors. Translating everything to
		// the FSA's local origin first keeps the arithmetic well
		// conditioned; area and overlap are translation-invariant, so only
		// the numerics change. fsaArea uses the cheap gross area (holes
		// counted positive) — it is only the floor's denominator, and the
		// exact even-odd polygonArea is prohibitively slow on archipelago
		// FSAs with hundreds of island contours.
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
			// cmas is sorted by UID ascending and we only replace on a
			// strictly larger area, so the smallest UID wins any tie
			// deterministically (real-data ties are effectively impossible).
			if area > bestArea {
				bestArea, bestUID = area, c.uid
			}
		}
		// Anchor only on a meaningful share of the FSA; a sub-threshold best
		// overlap is boundary line-work noise, so the FSA falls through to
		// its province (omitted from the map).
		if bestUID != "" && fsaArea > 0 && bestArea >= minOverlapFraction*fsaArea {
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
// Outer rings and holes are both passed as contours: polyclip interprets a
// polygon under the even-odd rule, so a ring nested inside another is
// treated as a hole regardless of its winding, and polygonArea measures the
// result the same way. Returns false for non-polygon or empty geometry.
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

// polygonArea returns the net area of a polyclip Polygon under the even-odd
// fill rule. polyclip's boolean output (connector.toPolygon) emits result
// contours in the order the sweep traced them and gives holes no guaranteed
// winding, so hole-ness cannot be inferred from each contour's signed-area
// sign — summing signed areas would add a hole's area instead of subtracting
// it. Instead a contour nested inside an odd number of other contours is a
// hole (subtracted); one nested in an even number, including zero, is filled
// (added). For valid (non-self-intersecting) even-odd polygons the result is
// non-negative.
func polygonArea(p polyclip.Polygon) float64 {
	ox, oy, ok := polygonOrigin(p)
	if !ok {
		return 0
	}
	// Precompute each contour's bounding box once so nestingDepth can reject
	// non-containing pairs in O(1); without this an archipelago FSA (hundreds
	// of disjoint island contours) costs O(contours² × vertices) in futile
	// point-in-polygon tests.
	bboxes := make([]polyclip.Rectangle, len(p))
	for i := range p {
		bboxes[i] = p[i].BoundingBox()
	}
	var total float64
	for i := range p {
		a := contourAbsArea(p[i], ox, oy)
		if a == 0 {
			continue
		}
		if nestingDepth(p, bboxes, i)%2 == 0 {
			total += a
		} else {
			total -= a
		}
	}
	return total
}

// translate returns a copy of p with every vertex shifted by (-dx,-dy),
// bringing it to a local origin so polyclip's sweep-line runs on
// well-conditioned (small) coordinates. Intersection area is
// translation-invariant, so results are unchanged — only the numerics
// improve. See assignFSAsToCMAs for why this matters at EPSG:3347 scale.
func translate(p polyclip.Polygon, dx, dy float64) polyclip.Polygon {
	out := make(polyclip.Polygon, len(p))
	for i, c := range p {
		nc := make(polyclip.Contour, len(c))
		for j, pt := range c {
			nc[j] = polyclip.Point{X: pt.X - dx, Y: pt.Y - dy}
		}
		out[i] = nc
	}
	return out
}

// polygonGrossArea sums the unsigned areas of every contour (holes counted
// positive), a cheap O(vertices) upper bound on the true area. It is the
// FSA-size denominator for the overlap floor, where a slight overestimate is
// harmless — unlike polygonArea it skips the O(contours²) even-odd nesting,
// which is prohibitively slow on archipelago FSAs with hundreds of island
// contours.
func polygonGrossArea(p polyclip.Polygon) float64 {
	ox, oy, ok := polygonOrigin(p)
	if !ok {
		return 0
	}
	var total float64
	for _, c := range p {
		total += contourAbsArea(c, ox, oy)
	}
	return total
}

// polygonOrigin returns the minimum-X/Y corner of p, used as a local origin
// for the area shoelace. StatsCan coordinates sit ~9e6 m from the EPSG:3347
// false origin, where the shoelace products reach ~1e13 and catastrophic
// float64 cancellation corrupts small areas (a sliver computes as noise
// instead of ~0). Subtracting this corner first keeps the arithmetic near
// zero; area is translation-invariant so well-scaled inputs are unchanged.
// ok is false for an empty polygon.
func polygonOrigin(p polyclip.Polygon) (ox, oy float64, ok bool) {
	ox, oy = math.Inf(1), math.Inf(1)
	for _, c := range p {
		for _, pt := range c {
			if pt.X < ox {
				ox = pt.X
			}
			if pt.Y < oy {
				oy = pt.Y
			}
		}
	}
	return ox, oy, !math.IsInf(ox, 1)
}

// contourAbsArea returns the unsigned area of a single contour via the
// shoelace formula, with every vertex shifted by the local origin (ox,oy) to
// avoid large-coordinate cancellation. Winding only flips the sign, which we
// discard: hole-ness is decided by nesting in polygonArea, not by winding.
func contourAbsArea(c polyclip.Contour, ox, oy float64) float64 {
	n := len(c)
	if n < 3 {
		return 0
	}
	var signed float64
	for i := range n {
		j := i + 1
		if j == n {
			j = 0
		}
		signed += (c[i].X-ox)*(c[j].Y-oy) - (c[j].X-ox)*(c[i].Y-oy)
	}
	if signed < 0 {
		signed = -signed
	}
	return signed / 2
}

// nestingDepth counts how many other contours of p contain contour i. The
// contours of a boolean-op result are pairwise non-crossing, so a point
// strictly interior to contour i lies strictly inside or strictly outside
// each other contour — one such point classifies the whole contour. Even
// depth ⇒ filled, odd ⇒ hole. The representative must be strictly interior,
// never a raw vertex: two result contours can touch at a vertex, and
// Contour.Contains classifies points on a contour's top/right edge as
// outside, so a shared boundary vertex would flip the parity (a hole counted
// as fill, or vice-versa). bboxes[j] is the precomputed bounding box of p[j];
// a representative outside it can't be contained, so the O(vertices) Contains
// is skipped.
func nestingDepth(p polyclip.Polygon, bboxes []polyclip.Rectangle, i int) int {
	if len(p[i]) < 3 {
		return 0
	}
	rep := interiorPoint(p[i])
	depth := 0
	for j := range p {
		if j == i || len(p[j]) < 3 {
			continue
		}
		bb := bboxes[j]
		if rep.X < bb.Min.X || rep.X > bb.Max.X || rep.Y < bb.Min.Y || rep.Y > bb.Max.Y {
			continue
		}
		if p[j].Contains(rep) {
			depth++
		}
	}
	return depth
}

// interiorPoint returns a point strictly inside the simple polygon contour c
// (len(c) >= 3), never on its boundary — the property nestingDepth needs so
// Contour.Contains can't misclassify it on another contour's half-open
// top/right edge. The lowest-then-leftmost vertex v is always a convex corner
// of c, so its interior wedge opens upward; the midpoint m of v's two
// neighbours gives m - v = ½(a-v) + ½(b-v), a strictly-positive combination of
// the two incident edge directions and thus a direction strictly inside that
// wedge. A hair's step from v toward m lands strictly inside c, off every
// edge. (Production calls this in FSA-local coordinates, so the step is well
// conditioned numerically.)
func interiorPoint(c polyclip.Contour) polyclip.Point {
	n := len(c)
	k := 0
	for i := 1; i < n; i++ {
		if c[i].Y < c[k].Y || (c[i].Y == c[k].Y && c[i].X < c[k].X) {
			k = i
		}
	}
	v := c[k]
	a := c[(k-1+n)%n]
	b := c[(k+1)%n]
	const eps = 1e-6
	return polyclip.Point{
		X: v.X + eps*((a.X+b.X)/2-v.X),
		Y: v.Y + eps*((a.Y+b.Y)/2-v.Y),
	}
}

// unionBBoxes returns the bounding box enclosing every box in boxes (a CMA's
// precomputed per-part boxes). Returns the zero Rectangle for an empty slice.
func unionBBoxes(boxes []polyclip.Rectangle) polyclip.Rectangle {
	if len(boxes) == 0 {
		return polyclip.Rectangle{}
	}
	box := boxes[0]
	for _, bb := range boxes[1:] {
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
