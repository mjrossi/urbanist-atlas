package ca

import (
	"math"

	polyclip "github.com/ctessum/polyclip-go"
)

// Stdlib-only planar geometry for the FSA → CMA spatial join, on
// polyclip's types. The numerical traps it avoids are written up in
// docs/architecture/etl-and-stores.md.

// polygonArea returns p's net area under the even-odd rule: a contour
// nested in an odd number of others is a hole. Signed-area sign can't be
// used, because polyclip's boolean output gives holes no guaranteed
// winding.
func polygonArea(p polyclip.Polygon) float64 {
	ox, oy, ok := polygonOrigin(p)
	if !ok {
		return 0
	}
	// Per-contour boxes let nestingDepth skip most pairs; archipelago FSAs
	// have hundreds of island contours.
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

// translate shifts every vertex of p by (-dx,-dy) so polyclip's
// sweep-line runs on small, well-conditioned coordinates.
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

// polygonGrossArea sums every contour's unsigned area, holes included:
// a cheap upper bound, good enough for the overlap floor's denominator,
// that skips polygonArea's quadratic nesting test.
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

// polygonOrigin returns p's minimum X/Y corner, a local origin for the
// shoelace: at EPSG:3347's ~9e6 m coordinates the products reach ~1e13
// and float64 cancellation turns small areas into noise. ok is false for
// an empty polygon.
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

// contourAbsArea returns a contour's unsigned shoelace area relative to
// (ox, oy).
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

// nestingDepth counts the contours of p that contain contour i. Boolean-op
// result contours never cross, so one strictly interior point classifies
// the whole contour. It must not be a vertex: contours can touch at one,
// and Contour.Contains treats top/right edges as outside, which would
// flip the parity.
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

// interiorPoint returns a point strictly inside contour c (len >= 3). The
// lowest-then-leftmost vertex is always convex, so a tiny step from it
// toward its neighbours' midpoint lands inside, off every edge.
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

// unionBBoxes returns the box enclosing boxes, or the zero Rectangle.
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

// bboxesOverlap reports whether two boxes intersect; touching counts.
func bboxesOverlap(a, b polyclip.Rectangle) bool {
	return a.Min.X <= b.Max.X && a.Max.X >= b.Min.X &&
		a.Min.Y <= b.Max.Y && a.Max.Y >= b.Min.Y
}
