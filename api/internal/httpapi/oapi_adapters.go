package httpapi

// Shared adapters between atlas domain types and the oapi-codegen wire
// shapes. Endpoint-specific adapters (toOAPILookupResult,
// toOAPIMetroSummaries, toOAPIMetroDetail) stay in their handler files;
// the helpers in this file are the ones that two or more endpoints
// share, so they have one canonical home.

import (
	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// toOAPIRegion adapts a domain Region onto the oapi wire shape. Used
// by /lookup (in resolved ancestry + each org's region list), /metros
// (the metro region itself), and /metros/{slug} (same).
func toOAPIRegion(r atlas.Region) oapi.Region {
	parentSlugs := r.ParentSlugs
	if parentSlugs == nil {
		parentSlugs = []string{}
	}
	return oapi.Region{
		Id:          r.ID,
		Kind:        oapi.RegionKind(r.Kind),
		Name:        r.Name,
		Slug:        r.Slug,
		Country:     oapi.Country(r.Country),
		ScopeTier:   oapi.ScopeTier(r.ScopeTier),
		ParentSlugs: parentSlugs,
	}
}

// toOAPIOrg adapts a domain Org onto the plain oapi.Org wire shape
// (used by /metros/{slug} and /recent). The /lookup endpoint uses
// oapi.LookupOrg, which extends this shape with MatchedRegionSlugs;
// see toOAPILookupOrg in lookup.go — it calls this helper and then
// attaches the lookup-specific field.
func toOAPIOrg(o atlas.Org) oapi.Org {
	tags := make([]string, len(o.Tags))
	for i, t := range o.Tags {
		tags[i] = string(t)
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
