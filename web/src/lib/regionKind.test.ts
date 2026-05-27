import { describe, expect, it } from 'vitest';
import {
  isBrowseableKind,
  isMetroKind,
  regionKindLabel,
} from './regionKind.ts';

describe('regionKind', () => {
  it('regionKindLabel returns a real label for the default browse set', () => {
    expect(regionKindLabel('us:metro')).toBe('Metropolitan area');
    expect(regionKindLabel('us:city')).toBe('City');
    expect(regionKindLabel('ca:cma')).toBe('Census Metropolitan Area');
  });

  it('isMetroKind is a strict subset of isBrowseableKind', () => {
    // Every kind we treat as a metro must also be browseable so the
    // SPA never tries to render a metro-tagged region as plain text
    // when /region/{slug} would resolve it.
    const knownKinds = [
      'us:metro',
      'us:city',
      'ca:cma',
      'ca:regional-district',
      'ca:city',
      'pt:area-metropolitana',
      'us:state',
      'us:multi-state',
      'us:county',
      'us:borough',
      'ca:province',
      'pt:municipio',
    ];
    for (const k of knownKinds) {
      if (isMetroKind(k) && !isBrowseableKind(k)) {
        throw new Error(`${k} is metro but not browseable`);
      }
    }
  });

  it('isBrowseableKind covers non-metro kinds the API resolves', () => {
    // States, multi-state coalitions, counties, boroughs, provinces
    // all return 200 from /api/v1/regions/{slug}; the Org detail page
    // must render their names as clickable links.
    expect(isBrowseableKind('us:state')).toBe(true);
    expect(isBrowseableKind('us:multi-state')).toBe(true);
    expect(isBrowseableKind('us:county')).toBe(true);
    expect(isBrowseableKind('us:borough')).toBe(true);
    expect(isBrowseableKind('ca:province')).toBe(true);
  });

  it('unknown kinds fall back to safe defaults', () => {
    expect(regionKindLabel('xx:unknown')).toBe('Region');
    expect(isBrowseableKind('xx:unknown')).toBe(false);
    expect(isMetroKind('xx:unknown')).toBe(false);
  });

  it('metro-equivalent kinds are flagged for primary-metro picks', () => {
    expect(isMetroKind('us:metro')).toBe(true);
    expect(isMetroKind('ca:cma')).toBe(true);
    expect(isMetroKind('pt:area-metropolitana')).toBe(true);
    // Cities are browseable but NOT metro-equivalent.
    expect(isMetroKind('us:city')).toBe(false);
    expect(isMetroKind('ca:city')).toBe(false);
  });
});
