# CA FSA → CMA spatial join: design notes

The reasoning behind `api/internal/etl/ca/spatial.go` and `geom.go`
that is too long for a code comment. The pipeline overview, including
where the join sits in the CA pipeline, lives in
[`docs/etl-architecture.md`](../etl-architecture.md).

## CA FSA → CMA: why max-overlap area, not point-in-polygon

`api/internal/etl/ca/spatial.go` assigns each Forward Sortation Area to
the Census Metropolitan Area it overlaps most by area, clipping the FSA
polygon against each CMA polygon with `polyclip`.

A simpler representative-point lookup was considered and rejected:
pick a point inside the FSA and ask which CMA contains it. That would
drop the polygon clipping, and with it the large-coordinate
translation, the even-odd area math and the noise floor, for roughly a
third less code. Area overlap was kept because it gives the principled
answer for the FSAs that motivate the code:

- An FSA straddling two adjacent metros anchors to the one it sits in
  *most*.
- A multi-part coastal FSA (BC's V8/V9 archipelagos) is assigned by
  total land overlap. Its representative point could land on an
  uninhabited islet.

The two approaches agree on the ~1,500 FSAs that sit cleanly inside or
outside a CMA. They differ only on the ~120 partial-overlap FSAs, where
"most of the FSA" is the defensible rule. Anyone tempted to simplify to
point-in-polygon should weigh that, not just the line count.

Both StatsCan boundary files are published in EPSG:3347 (NAD83
Statistics Canada Lambert, meters). The `.prj` inside each zip is
identical (same product series), so the code trusts it rather than
parsing the WKT, and planar areas compare directly.

## CA geometry: the numerical traps in `geom.go`

`geom.go` is stdlib-only planar geometry on `polyclip`'s types. Each
helper exists to avoid a specific failure:

- **Holes have no reliable winding.** `polyclip`'s boolean output
  (`connector.toPolygon`) emits contours in sweep order and gives holes
  no guaranteed winding, so summing signed areas can add a hole
  instead of subtracting it. `polygonArea` uses the even-odd rule: a
  contour nested inside an odd number of other contours is a hole.
- **Nesting needs a strictly interior point.** Result contours never
  cross, so one point strictly inside contour *i* is strictly inside or
  strictly outside every other contour, and that classifies the whole
  contour. A raw vertex won't do: two contours can touch at a vertex,
  and `Contour.Contains` treats points on a top or right edge as
  outside, which would flip the parity. `interiorPoint` takes the
  lowest-then-leftmost vertex *v*, which is always a convex corner, so
  its interior wedge opens upward. The midpoint *m* of *v*'s two
  neighbours gives *m − v = ½(a − v) + ½(b − v)*, a strictly positive
  combination of the two incident edge directions and so a direction
  strictly inside the wedge. A tiny step from *v* toward *m* lands
  inside the contour, off every edge. The step runs in FSA-local
  coordinates, so it is well conditioned.
- **Large coordinates lose precision.** EPSG:3347 coordinates sit about
  9e6 m from the false origin. There, the shoelace products reach about
  1e13 and float64 cancellation turns small areas into noise, and
  `polyclip`'s sweep-line mis-orders events and returns near-empty
  intersections for polygons that really overlap (an FSA wholly inside
  its CMA could compute as 0). The failure depends on the data, so it
  silently corrupts only some anchors. Both polygons are translated to
  the FSA's local origin before clipping, and every area is computed
  relative to a local origin. Area is translation-invariant, so only
  the numerics change.
- **Archipelagos are slow.** Exact even-odd area is quadratic in the
  number of contours, which hurts on FSAs with hundreds of islands.
  The FSA's own area is only the denominator of the noise floor, so it
  uses `polygonGrossArea`, which counts holes as positive: a cheap upper
  bound. Per-contour bounding boxes let `nestingDepth` skip most
  containment tests.

The noise floor (`minOverlapFraction = 0.001`) exists because StatsCan
digitizes FSA and CMA boundaries separately, leaving sub-square-meter
slivers along shared edges. In the 2021 vintage every sliver is at or
below 1e-6 of FSA area, and genuine partial overlaps start at 1e-3. The
floor sits in that empty gap. It removes noise only and does not impose
a "majority in the metro" rule.
