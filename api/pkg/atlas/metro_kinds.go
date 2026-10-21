package atlas

// metroKinds is the editorial set of metro-equivalent kinds (MSA, CMA,
// metropolitan area). placeLabel fills its "broad" slot from it; cities
// are excluded so a borough ZIP's label ends in the metro, not the city.
// See docs/region-graph.md, "Presentational kind sets".
var metroKinds = map[RegionKind]bool{
	"us:metro":              true,
	"ca:cma":                true,
	"ca:regional-district":  true,
	"pt:area-metropolitana": true,
}

// IsMetroKind reports whether k is one of the metro-equivalent kinds.
func IsMetroKind(k RegionKind) bool { return metroKinds[k] }
