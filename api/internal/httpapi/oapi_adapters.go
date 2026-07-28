package httpapi

// Adapters between atlas domain types and the oapi-codegen wire
// shapes. Every endpoint-specific adapter that converts from a
// pkg/atlas struct to its oapi.* counterpart lives here so the
// handler files stay thin (parse → call atlas → encode) and so the
// wire-shape surface is reviewed in one file.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// toOAPIRegion adapts a domain Region onto the oapi wire shape. Used
// by /lookup (in resolved ancestry + each org's region list), /regions
// (the region itself), and /regions/{slug} (same).
func toOAPIRegion(r atlas.Region) oapi.Region {
	return oapi.Region{
		Id:          r.ID,
		Kind:        oapi.RegionKind(r.Kind),
		Name:        r.Name,
		Slug:        r.Slug,
		Country:     oapi.Country(r.Country),
		ScopeTier:   oapi.ScopeTier(r.ScopeTier),
		ParentSlugs: nonNilSlice(r.ParentSlugs),
	}
}

// toOAPIOrg adapts a domain Org onto the plain oapi.Org wire shape
// (used by /regions/{slug} and /recent). The /lookup endpoint uses
// oapi.LookupOrg, which extends this shape with MatchedRegionSlugs;
// see toOAPILookupOrg below — it calls this helper and then attaches
// the lookup-specific field.
func toOAPIOrg(o atlas.Org) oapi.Org {
	tags := make([]string, 0, len(o.Tags))
	for _, t := range o.Tags {
		tags = append(tags, string(t))
	}
	regions := make([]oapi.Region, 0, len(o.Regions))
	for _, r := range o.Regions {
		regions = append(regions, toOAPIRegion(r))
	}
	out := oapi.Org{
		Id:         o.ID,
		Slug:       o.Slug,
		Name:       o.Name,
		ShortDesc:  o.ShortDesc,
		WebsiteUrl: o.WebsiteURL,
		Tags:       tags,
		Regions:    regions,
		AddedAt:    openapi_types.Date{Time: o.AddedAt},
	}
	if o.ContactURL != "" {
		cu := o.ContactURL
		out.ContactUrl = &cu
	}
	return out
}

// toOAPIOrgs lifts toOAPIOrg over a slice. Returns a non-nil
// zero-length slice when the input is empty so the JSON body is `[]`,
// not `null`.
func toOAPIOrgs(in []atlas.Org) []oapi.Org {
	out := make([]oapi.Org, 0, len(in))
	for _, o := range in {
		out = append(out, toOAPIOrg(o))
	}
	return out
}

// toOAPILookupResult adapts the atlas package's native result type
// onto the oapi-generated wire type. The JSON shapes are identical;
// this adapter is a typed conversion so the handler signature is
// "returns oapi.LookupResult", which keeps the wire contract front and
// center in code review.
func toOAPILookupResult(in atlas.LookupResult) oapi.LookupResult {
	ancestry := make([]oapi.Region, 0, len(in.ResolvedAncestry))
	for _, r := range in.ResolvedAncestry {
		ancestry = append(ancestry, toOAPIRegion(r))
	}
	return oapi.LookupResult{
		Query: oapi.LookupQuery{
			PostalCode: in.Query.PostalCode,
			Country:    oapi.Country(in.Query.Country),
		},
		ResolvedPlaceLabel: in.ResolvedPlaceLabel,
		ResolvedAncestry:   ancestry,
		Local:              toOAPILookupOrgs(in.Local),
		Regional:           toOAPILookupOrgs(in.Regional),
		Statewide:          toOAPILookupOrgs(in.Statewide),
	}
}

// toOAPILookupOrgs lifts toOAPILookupOrg over a slice. Returns a
// non-nil zero-length slice when the input is empty so the JSON body
// is `[]`, not `null`.
func toOAPILookupOrgs(orgs []atlas.Org) []oapi.LookupOrg {
	out := make([]oapi.LookupOrg, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, toOAPILookupOrg(o))
	}
	return out
}

// toOAPILookupOrg builds the /lookup-specific wire shape on top of the
// shared toOAPIOrg adapter. The only lookup-specific field is
// MatchedRegionSlugs, which the lookup algorithm in pkg/atlas
// computes per org.
func toOAPILookupOrg(o atlas.Org) oapi.LookupOrg {
	base := toOAPIOrg(o)
	return oapi.LookupOrg{
		Id:                 base.Id,
		Slug:               base.Slug,
		Name:               base.Name,
		ShortDesc:          base.ShortDesc,
		WebsiteUrl:         base.WebsiteUrl,
		Tags:               base.Tags,
		Regions:            base.Regions,
		ContactUrl:         base.ContactUrl,
		AddedAt:            base.AddedAt,
		MatchedRegionSlugs: nonNilSlice(o.MatchedRegionSlugs),
	}
}

// toOAPIRegionSummaries converts the domain-level region list to the
// wire-level slice. Returns a non-nil zero-length slice when the
// input is empty so the JSON body is `[]`, not `null`.
//
// Empty BrowseParentSlug maps to JSON null (omitempty pointer); a
// non-empty value renders as a string. Lets the SPA group cities
// under their parent metro without a second request.
func toOAPIRegionSummaries(in []atlas.RegionSummary) []oapi.RegionSummary {
	out := make([]oapi.RegionSummary, 0, len(in))
	for _, rs := range in {
		summary := oapi.RegionSummary{
			Region:         toOAPIRegion(rs.Region),
			OrgCount:       int32(rs.OrgCount),
			DirectOrgCount: int32(rs.DirectOrgCount),
		}
		if rs.BrowseParentSlug != "" {
			s := rs.BrowseParentSlug
			summary.BrowseParentSlug = &s
		}
		out = append(out, summary)
	}
	return out
}

// toOAPIStats converts the domain-level atlas summary to the wire
// shape. ByCountry is forced non-nil so the JSON body carries `[]`
// rather than `null` on an empty store.
func toOAPIStats(in atlas.Stats) oapi.Stats {
	byCountry := make([]oapi.CountryStats, 0, len(in.ByCountry))
	for _, c := range in.ByCountry {
		byCountry = append(byCountry, oapi.CountryStats{
			Country:     oapi.Country(c.Country),
			OrgCount:    int32(c.OrgCount),
			RegionCount: int32(c.RegionCount),
		})
	}
	return oapi.Stats{
		TotalOrgCount:     int32(in.TotalOrgCount),
		TotalRegionCount:  int32(in.TotalRegionCount),
		BrowseRegionCount: int32(in.BrowseRegionCount),
		ByCountry:         byCountry,
	}
}

// toOAPIRegionSearchResults converts the domain-level search results to
// the wire-level slice. Returns a non-nil zero-length slice when the
// input is empty so the JSON body is `[]`, not `null`. ContextLabel
// maps to a plain (possibly empty) string — the disambiguation hint is
// required on the wire, never null.
func toOAPIRegionSearchResults(in []atlas.RegionSearchResult) []oapi.RegionSearchResult {
	out := make([]oapi.RegionSearchResult, 0, len(in))
	for _, rsr := range in {
		out = append(out, oapi.RegionSearchResult{
			Region:       toOAPIRegion(rsr.Region),
			ContextLabel: rsr.ContextLabel,
		})
	}
	return out
}

// toOAPIRegionDetail converts a single domain region to the wire
// shape. Orgs are bucketed by attachment scope_tier (the shared
// pkg/atlas helper) and mapped via toOAPILookupOrgs so each row
// carries its matched_region_slugs — same shape /lookup returns,
// driving the same SPA components.
//
// Ancestry mirrors the closest-first walk pkg/atlas built (direct
// parent at index 0, root at the end, national-tier rows filtered).
// The SPA renders it as a breadcrumb in the Region page kicker.
func toOAPIRegionDetail(in atlas.RegionDetail) oapi.RegionDetail {
	ancestry := make([]oapi.Region, 0, len(in.Ancestry))
	for _, r := range in.Ancestry {
		ancestry = append(ancestry, toOAPIRegion(r))
	}
	names := in.DescendantRegionNames
	if names == nil {
		names = map[string]string{}
	}
	return oapi.RegionDetail{
		Region:                toOAPIRegion(in.Region),
		Local:                 toOAPILookupOrgs(in.Local),
		Regional:              toOAPILookupOrgs(in.Regional),
		Statewide:             toOAPILookupOrgs(in.Statewide),
		Ancestry:              ancestry,
		DescendantRegionNames: names,
	}
}
