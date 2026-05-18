/**
 * Typed dev fixtures for slice #14 (browse + metro pages, homepage
 * asides). Lets the SPA build and tests pass while the backend slice
 * (#6, parallel worktree) is in flight.
 *
 * Every export is typed against `components['schemas']` so any drift
 * from the real wire shape is a compile error. Values mirror entries
 * in `api/seed/regions_*.toml` and `api/seed/orgs.toml` so the dev
 * experience matches what the real backend will return.
 *
 * **This file is deleted in the post-backend-merge cleanup commit,
 * along with the `VITE_USE_FIXTURES` branches in `api.ts`.**
 */
import type { components } from '../api.gen.ts';

type MetroSummary = components['schemas']['MetroSummary'];
type MetroDetail = components['schemas']['MetroDetail'];
type Org = components['schemas']['Org'];
type Region = components['schemas']['Region'];

const nycMetro: Region = {
  id: 101,
  kind: 'us:metro',
  name: 'New York Metro',
  slug: 'nyc-metro',
  country: 'US',
  scope_tier: 'regional',
  parent_slugs: ['nyc-tristate'],
};

const sfBayArea: Region = {
  id: 102,
  kind: 'us:metro',
  name: 'San Francisco Bay Area',
  slug: 'sf-bay-area',
  country: 'US',
  scope_tier: 'regional',
  parent_slugs: [],
};

const greaterBoston: Region = {
  id: 103,
  kind: 'us:metro',
  name: 'Greater Boston',
  slug: 'greater-boston',
  country: 'US',
  scope_tier: 'regional',
  parent_slugs: [],
};

const torontoCma: Region = {
  id: 104,
  kind: 'ca:cma',
  name: 'Greater Toronto Area',
  slug: 'toronto-cma',
  country: 'CA',
  scope_tier: 'regional',
  parent_slugs: ['on'],
};

const metroVancouver: Region = {
  id: 105,
  kind: 'ca:regional-district',
  name: 'Metro Vancouver',
  slug: 'metro-vancouver',
  country: 'CA',
  scope_tier: 'regional',
  parent_slugs: ['bc'],
};

const aml: Region = {
  id: 106,
  kind: 'pt:area-metropolitana',
  name: 'Área Metropolitana de Lisboa',
  slug: 'aml',
  country: 'PT',
  scope_tier: 'regional',
  parent_slugs: ['nuts-ii-grande-lisboa', 'nuts-ii-peninsula-setubal'],
};

/**
 * Metros for the `/browse` page and the homepage "Browse by metro"
 * aside. Ordered descending by `org_count` to mirror the real API's
 * sort order (the page does not re-sort).
 */
export const metrosFixture: MetroSummary[] = [
  { region: nycMetro, org_count: 12 },
  { region: sfBayArea, org_count: 7 },
  { region: torontoCma, org_count: 5 },
  { region: greaterBoston, org_count: 4 },
  { region: aml, org_count: 3 },
  { region: metroVancouver, org_count: 2 },
];

/**
 * Detail fixtures keyed by slug. Only the metros plausibly visited
 * during fixture-mode dev get a full org list; unknown slugs fall
 * through to a synthetic 404 in {@link getMetro}.
 */
export const metroDetailFixture: Record<string, MetroDetail> = {
  'nyc-metro': {
    region: nycMetro,
    orgs: [
      {
        id: 1001,
        slug: 'transitcenter',
        name: 'TransitCenter',
        short_desc:
          'Foundation and research outfit working to improve public transit in cities across the US; NYC-based, NYC-metro reach.',
        website_url: 'https://transitcenter.org',
        tags: ['transit', 'policy', 'research'],
        regions: [nycMetro],
      },
      {
        id: 1002,
        slug: 'transportation-alternatives',
        name: 'Transportation Alternatives',
        short_desc:
          "NYC's largest streets-and-mobility advocacy organization, pushing for safer streets, better transit, and protected bike infrastructure.",
        website_url: 'https://transalt.org',
        contact_url: 'https://transalt.org/contact',
        tags: ['advocacy', 'safe-streets', 'cycling', 'walking', 'vision-zero'],
        regions: [nycMetro],
      },
      {
        id: 1003,
        slug: 'riders-alliance',
        name: 'Riders Alliance',
        short_desc:
          'Grassroots organization of NYC transit riders fighting for more reliable, affordable, and accessible subways and buses.',
        website_url: 'https://www.ridersny.org',
        tags: ['transit', 'grassroots'],
        regions: [nycMetro],
      },
    ],
  },
  aml: {
    region: aml,
    orgs: [
      {
        id: 1101,
        slug: 'mubi',
        name: 'MUBi',
        short_desc:
          'Associação pela mobilidade urbana em bicicleta — Portuguese cycling advocacy nonprofit.',
        website_url: 'https://mubi.pt',
        tags: ['cycling', 'advocacy'],
        regions: [aml],
      },
      {
        id: 1102,
        slug: 'zero',
        name: 'ZERO — Associação Sistema Terrestre Sustentável',
        short_desc:
          'Portuguese environmental advocacy organization with a strong sustainable mobility programme.',
        website_url: 'https://zero.ong',
        tags: ['policy', 'advocacy'],
        regions: [aml],
      },
    ],
  },
};

/**
 * Recent fixture orgs for the homepage "Recently added" aside.
 * Five entries matches the slice's design target for that card.
 */
export const recentFixture: Org[] = [
  {
    id: 2001,
    slug: 'streets-for-all',
    name: 'Streets For All',
    short_desc:
      'Los Angeles-based advocacy organization for safer, more sustainable streets and transit across the LA region.',
    website_url: 'https://www.streetsforall.org',
    tags: ['advocacy', 'safe-streets', 'transit', 'cycling'],
    regions: [],
  },
  {
    id: 2002,
    slug: 'seattle-subway',
    name: 'Seattle Subway',
    short_desc:
      'Volunteer advocacy group pushing for an expanded, faster-built rail transit network across the Seattle region.',
    website_url: 'https://seattlesubway.org',
    tags: ['transit', 'grassroots', 'rail'],
    regions: [],
  },
  {
    id: 2003,
    slug: 'transit-alliance-miami',
    name: 'Transit Alliance Miami',
    short_desc:
      'Grassroots advocacy organization working to make Miami a more walkable, bikeable, transit-friendly community.',
    website_url: 'https://transitalliance.miami',
    tags: ['transit', 'advocacy', 'safe-streets'],
    regions: [],
  },
  {
    id: 2004,
    slug: 'livablestreets-alliance',
    name: 'LivableStreets Alliance',
    short_desc:
      'Cambridge-based nonprofit advocating for innovative transportation solutions and complete streets across Greater Boston.',
    website_url: 'https://www.livablestreets.info',
    tags: ['safe-streets', 'transit', 'cycling', 'walking'],
    regions: [],
  },
  {
    id: 2005,
    slug: 'sf-transit-riders',
    name: 'San Francisco Transit Riders',
    short_desc:
      'Member-driven advocacy organization fighting for excellent public transit in San Francisco.',
    website_url: 'https://www.sftransitriders.org',
    tags: ['transit', 'grassroots'],
    regions: [],
  },
];
