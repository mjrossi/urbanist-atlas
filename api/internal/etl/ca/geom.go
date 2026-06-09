package ca

import (
	"math"

	polyclip "github.com/ctessum/polyclip-go"
)

// geom.go holds the hand-rolled planar-geometry primitives the FSA → CMA
// spatial join (spatial.go) builds on: even-odd polygon area, the local-
// origin shoelace, contour-nesting classification, a strictly-interior
// representative point, polygon translation, and the bounding-box math.
// They are deliberately stdlib-only (no geo library) and operate on
// polyclip's Polygon/Contour/Rectangle types. spatial.go keeps the
// shapefile load + join orchestration that calls these.

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
// neighbors gives m - v = ½(a-v) + ½(b-v), a strictly-positive combination of
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
