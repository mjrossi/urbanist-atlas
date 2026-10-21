package atlas

// stateKinds is the editorial set of state-equivalent (top
// administrative tier) kinds: BucketOrgsByScope puts an org matched at
// one of them in the Statewide tier. DC and multi-state coalitions are
// deliberately out; see docs/region-graph.md, "Presentational kind sets".
var stateKinds = map[RegionKind]bool{
	"us:state":     true,
	"us:territory": true,
	"ca:province":  true,
	"ca:territory": true,
}

// IsStateKind reports whether k is one of the state-equivalent (top
// administrative tier) kinds.
func IsStateKind(k RegionKind) bool { return stateKinds[k] }

// IsRollupTargetKind reports whether k may be named in a region's
// rollup_states: the state-equivalent kinds plus us:federal-district, so
// the bi-state Washington metro can surface its own orgs on /dc.
func IsRollupTargetKind(k RegionKind) bool {
	return stateKinds[k] || k == "us:federal-district"
}
