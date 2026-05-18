package atlas

// MetroSummary is the domain shape returned by Store.ListMetros: one
// metro region and the number of approved organizations that serve it
// (directly or via the region DAG). The wire-level twin lives in the
// generated oapi package; the HTTP handler adapts one to the other so
// pkg/atlas stays free of OpenAPI types.
type MetroSummary struct {
	Region   Region
	OrgCount int64
}

// MetroDetail is the domain shape returned by Store.GetMetro: a metro
// region plus the approved orgs that serve it (directly or via the
// region DAG). Orgs is newest-first when the underlying store has a
// creation timestamp; the MemStore tolerates the field being zero.
type MetroDetail struct {
	Region Region
	Orgs   []Org
}
