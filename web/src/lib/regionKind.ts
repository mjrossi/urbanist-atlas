/**
 * Per-kind UI metadata. `label` keeps taxonomy strings like `us:metro`
 * out of copy; `browseable` means `/region/{slug}` resolves the kind, so
 * the name renders as a link (the API 404s national-tier slugs, so
 * national kinds stay out of this map); `metro` marks the kinds
 * {@link primaryRegion} prefers.
 */
interface RegionKindInfo {
  label: string;
  browseable: boolean;
  metro: boolean;
}

const REGION_KINDS: Record<string, RegionKindInfo> = {
  // The `/api/v1/regions` browse set (`defaultBrowseKinds` in the API).
  'us:metro': { label: 'Metropolitan area', browseable: true, metro: true },
  'us:city': { label: 'City', browseable: true, metro: false },
  'ca:cma': { label: 'Census Metropolitan Area', browseable: true, metro: true },
  'ca:regional-district': { label: 'Regional district', browseable: true, metro: true },
  'ca:city': { label: 'City', browseable: true, metro: false },
  'pt:area-metropolitana': { label: 'Área Metropolitana', browseable: true, metro: true },

  // Outside the browse set, but reachable at /region/{slug}.
  'us:state': { label: 'State', browseable: true, metro: false },
  'us:federal-district': { label: 'Federal district', browseable: true, metro: false },
  'us:territory': { label: 'Territory', browseable: true, metro: false },
  'us:county': { label: 'County', browseable: true, metro: false },
  'us:borough': { label: 'Borough', browseable: true, metro: false },
  'us:multi-state': { label: 'Multi-state region', browseable: true, metro: false },
  'us:transit-federation': {
    label: 'Transit federation',
    browseable: true,
    metro: false,
  },
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

// One dev warning per unknown kind per session.
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

/**
 * The region to name an org by: a metro-equivalent kind if it has one,
 * then the lowest slug. `org.regions` is unordered on the wire, so this
 * must not depend on array order.
 */
export function primaryRegion<R extends { kind: string; slug: string }>(
  regions: readonly R[],
): R | undefined {
  let best: R | undefined;
  for (const r of regions) {
    if (!best) {
      best = r;
      continue;
    }
    const rMetro = isMetroKind(r.kind);
    const bestMetro = isMetroKind(best.kind);
    if (rMetro !== bestMetro ? rMetro : r.slug < best.slug) best = r;
  }
  return best;
}
