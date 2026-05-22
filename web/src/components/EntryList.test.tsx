import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { EntryList } from './EntryList.tsx';
import type { LookupOrg } from '../lib/api.ts';

// EntryList renders <Entry>, which now uses `<Link>` for the org
// name. Wrap each render in a MemoryRouter so the router context
// resolves; the asserted behavior is unchanged.
function renderList(node: React.ReactNode) {
  return render(<MemoryRouter>{node}</MemoryRouter>);
}

function makeOrg(id: number, name: string, tags: string[] = []): LookupOrg {
  return {
    id,
    slug: `org-${id}`,
    name,
    short_desc: `${name} short description`,
    website_url: `https://example.com/${id}`,
    tags,
    regions: [],
    matched_region_slugs: [],
  };
}

const emptyMap = new Map<string, string>();

describe('EntryList', () => {
  it('renders both Local and Regional section labels', () => {
    renderList(<EntryList local={[]} regional={[]} regionNameBySlug={emptyMap} />);
    expect(screen.getByText('Local')).toBeDefined();
    expect(screen.getByText('Regional')).toBeDefined();
  });

  it('renders entries inside the matching section', () => {
    renderList(
      <EntryList
        local={[makeOrg(1, 'Local Org A'), makeOrg(2, 'Local Org B')]}
        regional={[makeOrg(3, 'Regional Org C')]}
        regionNameBySlug={emptyMap}
      />,
    );
    expect(screen.getByRole('link', { name: 'Local Org A' })).toBeDefined();
    expect(screen.getByRole('link', { name: 'Local Org B' })).toBeDefined();
    expect(screen.getByRole('link', { name: 'Regional Org C' })).toBeDefined();
  });

  it('shows a per-section empty hint when a tier is empty', () => {
    renderList(
      <EntryList local={[]} regional={[makeOrg(1, 'Only Regional')]} regionNameBySlug={emptyMap} />,
    );
    expect(screen.getByText('No local groups indexed yet.')).toBeDefined();
    expect(screen.queryByText('No regional groups indexed yet.')).toBeNull();
  });
});
