package atlas

// defaultBrowseKinds is the editorial set of kinds `/api/v1/regions`
// returns: metros and cities. It is a superset of metroKinds; see
// docs/region-graph.md, "Presentational kind sets".
var defaultBrowseKinds = map[RegionKind]bool{
	"us:metro":              true,
	"us:city":               true,
	"ca:cma":                true,
	"ca:regional-district":  true,
	"ca:city":               true,
	"pt:area-metropolitana": true,
}

// IsDefaultBrowseKind reports whether k is one of the kinds returned
// by `/api/v1/regions`.
func IsDefaultBrowseKind(k RegionKind) bool { return defaultBrowseKinds[k] }
