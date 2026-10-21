package httpapi

// Adapters from pkg/atlas types to the oapi-codegen wire shapes, kept in
// one file so the wire surface is reviewed in one place.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

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

func toOAPIOrgs(in []atlas.Org) []oapi.Org {
	out := make([]oapi.Org, 0, len(in))
	for _, o := range in {
		out = append(out, toOAPIOrg(o))
	}
	return out
}

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

func toOAPILookupOrgs(orgs []atlas.Org) []oapi.LookupOrg {
	out := make([]oapi.LookupOrg, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, toOAPILookupOrg(o))
	}
	return out
}

// toOAPILookupOrg is toOAPIOrg plus MatchedRegionSlugs.
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

// toOAPIRegionSummaries omits an empty BrowseParentSlug from the wire.
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

// toOAPIRegionDetail maps orgs to the same LookupOrg shape /lookup
// returns, matched_region_slugs included.
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
