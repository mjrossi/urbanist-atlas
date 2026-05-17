package atlas

// LoadDevFixtures populates a MemStore with a small set of seeded
// organizations across three urban areas (NYC, SF Bay, Toronto). It
// exists so `cmd/server serve` can render a working /api/v1/lookup
// before the Postgres store is wired up.
//
// The fixture data here is illustrative — not authoritative. Real
// seed data lives in api/seed/orgs.yaml once that pipeline lands.
//
//nolint:funlen // long but flat; readability beats decomposition for a fixture loader.
func LoadDevFixtures(s *MemStore) {
	// ── Regions ────────────────────────────────────────────────
	// NYC area
	s.AddRegion(Region{ID: 1, Kind: RegionCity, Name: "Brooklyn", Slug: "brooklyn-ny", Country: CountryUS, ScopeTier: ScopeLocal})
	s.AddRegion(Region{ID: 2, Kind: RegionCounty, Name: "Kings County, NY", Slug: "kings-county-ny", Country: CountryUS, ScopeTier: ScopeLocal})
	s.AddRegion(Region{ID: 3, Kind: RegionMetro, Name: "New York Metro", Slug: "nyc-metro", Country: CountryUS, ScopeTier: ScopeRegional})
	s.AddRegion(Region{ID: 4, Kind: RegionState, Name: "NY", Slug: "ny", Country: CountryUS, ScopeTier: ScopeRegional})

	// SF Bay
	s.AddRegion(Region{ID: 10, Kind: RegionCity, Name: "San Francisco", Slug: "san-francisco-ca", Country: CountryUS, ScopeTier: ScopeLocal})
	s.AddRegion(Region{ID: 11, Kind: RegionCounty, Name: "San Francisco County", Slug: "san-francisco-county-ca", Country: CountryUS, ScopeTier: ScopeLocal})
	s.AddRegion(Region{ID: 12, Kind: RegionMetro, Name: "SF Bay Area", Slug: "sf-bay-area", Country: CountryUS, ScopeTier: ScopeRegional})
	s.AddRegion(Region{ID: 13, Kind: RegionState, Name: "CA", Slug: "ca", Country: CountryUS, ScopeTier: ScopeRegional})

	// Toronto
	s.AddRegion(Region{ID: 20, Kind: RegionCity, Name: "Toronto", Slug: "toronto-on", Country: CountryCA, ScopeTier: ScopeLocal})
	s.AddRegion(Region{ID: 21, Kind: RegionMetro, Name: "Toronto CMA", Slug: "toronto-cma", Country: CountryCA, ScopeTier: ScopeRegional})
	s.AddRegion(Region{ID: 22, Kind: RegionProvince, Name: "Ontario", Slug: "ontario", Country: CountryCA, ScopeTier: ScopeRegional})

	// ── Postal codes ───────────────────────────────────────────
	bk := s.regions[1]
	kings := s.regions[2]
	nycMetro := s.regions[3]
	ny := s.regions[4]
	sf := s.regions[10]
	sfCo := s.regions[11]
	bay := s.regions[12]
	ca := s.regions[13]
	tor := s.regions[20]
	torCMA := s.regions[21]
	ont := s.regions[22]

	s.AddPostalCode(ResolvedPostalCode{Code: "11217", Country: CountryUS, City: &bk, County: &kings, Metro: &nycMetro, State: &ny})
	s.AddPostalCode(ResolvedPostalCode{Code: "11215", Country: CountryUS, City: &bk, County: &kings, Metro: &nycMetro, State: &ny})
	s.AddPostalCode(ResolvedPostalCode{Code: "94110", Country: CountryUS, City: &sf, County: &sfCo, Metro: &bay, State: &ca})
	s.AddPostalCode(ResolvedPostalCode{Code: "M5V", Country: CountryCA, City: &tor, Metro: &torCMA, State: &ont})

	// ── Organizations ──────────────────────────────────────────
	// NYC
	s.AddOrg(Org{
		ID: 1, Slug: "transalt-brooklyn",
		Name:       "Transportation Alternatives — Brooklyn",
		ShortDesc:  "The Brooklyn committee of NYC's largest streets-and-mobility advocacy organization.",
		WebsiteURL: "https://www.transalt.org",
		Tags:       []Tag{"safe-streets", "cycling", "vision-zero"},
	}, []int64{1, 2})

	s.AddOrg(Org{
		ID: 2, Slug: "brooklyn-spoke",
		Name:       "Brooklyn Spoke",
		ShortDesc:  "Volunteer-led neighborhood group focused on traffic calming and pedestrian safety in central Brooklyn.",
		WebsiteURL: "https://brooklynspoke.com",
		Tags:       []Tag{"safe-streets", "neighborhood"},
	}, []int64{1})

	s.AddOrg(Org{
		ID: 3, Slug: "riders-alliance",
		Name:       "Riders Alliance",
		ShortDesc:  "Grassroots organization of NYC transit riders fighting for more reliable, affordable, and accessible subways and buses.",
		WebsiteURL: "https://www.ridersny.org",
		Tags:       []Tag{"transit", "grassroots"},
	}, []int64{3})

	s.AddOrg(Org{
		ID: 4, Slug: "tri-state-transportation-campaign",
		Name:       "Tri-State Transportation Campaign",
		ShortDesc:  "Policy-focused coalition advocating for sustainable transportation across New York, New Jersey, and Connecticut.",
		WebsiteURL: "https://tstc.org",
		Tags:       []Tag{"transit", "policy"},
	}, []int64{4})

	s.AddOrg(Org{
		ID: 5, Slug: "streetspac",
		Name:       "StreetsPAC",
		ShortDesc:  "Political action committee endorsing candidates for NYC offices based on their record on safe streets and transit.",
		WebsiteURL: "https://streetspac.org",
		Tags:       []Tag{"political"},
	}, []int64{3})

	// SF Bay
	s.AddOrg(Org{
		ID: 10, Slug: "sf-transit-riders",
		Name:       "San Francisco Transit Riders",
		ShortDesc:  "Member-driven advocacy organization fighting for excellent public transit in San Francisco.",
		WebsiteURL: "https://www.sftransitriders.org",
		Tags:       []Tag{"transit", "grassroots"},
	}, []int64{10, 11})

	s.AddOrg(Org{
		ID: 11, Slug: "walk-sf",
		Name:       "Walk San Francisco",
		ShortDesc:  "Pedestrian advocacy organization working to make San Francisco's streets safer for people walking.",
		WebsiteURL: "https://walksf.org",
		Tags:       []Tag{"walking", "safe-streets", "vision-zero"},
	}, []int64{10})

	s.AddOrg(Org{
		ID: 12, Slug: "spur",
		Name:       "SPUR",
		ShortDesc:  "Bay Area policy organization promoting good planning and good government.",
		WebsiteURL: "https://www.spur.org",
		Tags:       []Tag{"policy"},
	}, []int64{12})

	// Toronto
	s.AddOrg(Org{
		ID: 20, Slug: "ttcriders",
		Name:       "TTCriders",
		ShortDesc:  "Grassroots membership-based organization advocating for better transit in Toronto.",
		WebsiteURL: "https://ttcriders.ca",
		Tags:       []Tag{"transit", "grassroots"},
	}, []int64{20})

	s.AddOrg(Org{
		ID: 21, Slug: "walk-toronto",
		Name:       "Walk Toronto",
		ShortDesc:  "Volunteer pedestrian advocacy group working for safer, more walkable streets across Toronto.",
		WebsiteURL: "https://walktoronto.ca",
		Tags:       []Tag{"walking", "safe-streets"},
	}, []int64{20})
}
