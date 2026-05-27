/**
 * Region-kind metadata: one source of truth for the human-readable
 * label AND the two predicates the SPA uses to decide UI affordances.
 *
 * - `label`: shown in prose so taxonomy strings (`us:metro`,
 *   `us:multi-state`, …) never leak into UI copy.
 * - `browseable`: true if `/region/{slug}` resolves the kind, so the
 *   SPA should render the name as a Link. Every kind in this map is
 *   non-national (the server returns 404 for `scope_tier='national'`
 *   slugs); editorial policy keeps national kinds out of the map
 *   entirely.
 * - `metro`: true if the kind is metro-equivalent — the "primary
 *   metro" rail on org pages picks the first region with this set.
 *
 * The default browse set (kinds returned by `/api/v1/regions`) is
 * `browseable && (metro || …)` — those are the metros + cities the
 * Browse index ships. The broader `browseable: true` set also
 * includes states, counties, multi-state regions, etc. that are
 * reachable by typing `/region/<slug>` directly.
 *
 * Adding a new country's kinds means editing this one record, not
 * three independent sets across the SPA.
 */
type RegionKindInfo = {
  label: string;
  browseable: boolean;
  metro: boolean;
};

const REGION_KINDS: Record<string, RegionKindInfo> = {
  // Default browse set: metros + cities, surfaced by /api/v1/regions.
  'us:metro': { label: 'Metropolitan area', browseable: true, metro: true },
  'us:city': { label: 'City', browseable: true, metro: false },
  'ca:cma': { label: 'Census Metropolitan Area', browseable: true, metro: true },
  'ca:regional-district': { label: 'Regional district', browseable: true, metro: true },
  'ca:city': { label: 'City', browseable: true, metro: false },
  'pt:area-metropolitana': { label: 'Área Metropolitana', browseable: true, metro: true },

  // Surfaced via /regions/{slug} when navigating directly to a graph
  // node outside the default browse set. All non-national, so all
  // browseable; none metro-equivalent.
  'us:state': { label: 'State', browseable: true, metro: false },
  'us:federal-district': { label: 'Federal district', browseable: true, metro: false },
  'us:territory': { label: 'Territory', browseable: true, metro: false },
  'us:county': { label: 'County', browseable: true, metro: false },
  'us:borough': { label: 'Borough', browseable: true, metro: false },
  'us:multi-state': { label: 'Multi-state region', browseable: true, metro: false },
  'us:transit-federation': { label: 'Transit federation', browseable: true, metro: false },
  'ca:province': { label: 'Province', browseable: true, metro: false },
  'ca:territory': { label: 'Territory', browseable: true, metro: false },
  'pt:distrito': { label: 'Distrito', browseable: true, metro: false },
  'pt:cim': { label: 'Comunidade Intermunicipal', browseable: true, metro: false },
  'pt:municipio': { label: 'Município', browseable: true, metro: false },
  'pt:freguesia': { label: 'Freguesia', browseable: true, metro: false },
  'pt:nuts-ii': { label: 'NUTS II', browseable: true, metro: false },
  'pt:regiao-autonoma': { label: 'Região Autónoma', browseable: true, metro: false },
};

const UNKNOWN: RegionKindInfo = { label: 'Region', browseable: false, metro: false };

// Track kinds we've already warned about so dev-mode console spam stays
// bounded — schema drift only needs to flag once per kind per session.
const warnedKinds = new Set<string>();

function infoFor(kind: string): RegionKindInfo {
  const info = REGION_KINDS[kind];
  if (!info && import.meta.env.DEV && !warnedKinds.has(kind)) {
    warnedKinds.add(kind);
    console.warn(
      `[regionKind] unknown kind "${kind}" — add to REGION_KINDS in src/lib/regionKind.ts`,
    );
  }
  return info ?? UNKNOWN;
}

export function regionKindLabel(kind: string): string {
  return infoFor(kind).label;
}

export function isBrowseableKind(kind: string): boolean {
  return infoFor(kind).browseable;
}

export function isMetroKind(kind: string): boolean {
  return infoFor(kind).metro;
}
