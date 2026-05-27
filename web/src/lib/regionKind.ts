/**
 * Human-readable labels for `region.kind` values. Used by the Browse
 * index and the Region detail page so the kind reads naturally in
 * prose without exposing the underlying taxonomy string
 * (`us:metro`, `us:multi-state`, etc.) in UI copy.
 *
 * The map covers the default browse set (metros + cities) and the
 * broader set the detail endpoint now resolves (states, counties,
 * boroughs, multi-state regions, transit federations). Unknown kinds
 * fall back to "Region" — defensive against future country additions
 * that hit the SPA before this map is updated.
 */
const labels: Record<string, string> = {
  // Default browse set
  'us:metro': 'Metropolitan area',
  'us:city': 'City',
  'ca:cma': 'Census Metropolitan Area',
  'ca:regional-district': 'Regional district',
  'ca:city': 'City',
  'pt:area-metropolitana': 'Área Metropolitana',

  // Surfaced via /regions/{slug} when navigating directly to a graph
  // node outside the default browse set.
  'us:state': 'State',
  'us:federal-district': 'Federal district',
  'us:territory': 'Territory',
  'us:county': 'County',
  'us:borough': 'Borough',
  'us:multi-state': 'Multi-state region',
  'us:transit-federation': 'Transit federation',
  'ca:province': 'Province',
  'ca:territory': 'Territory',
  'pt:distrito': 'Distrito',
  'pt:cim': 'Comunidade Intermunicipal',
  'pt:municipio': 'Município',
  'pt:freguesia': 'Freguesia',
  'pt:nuts-ii': 'NUTS II',
  'pt:regiao-autonoma': 'Região Autónoma',
};

export function regionKindLabel(kind: string): string {
  return labels[kind] ?? 'Region';
}
