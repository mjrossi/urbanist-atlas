import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { EntryList } from './EntryList.tsx';
import type { LookupOrg } from '../lib/api.ts';

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
  it('renders the Local section label only when local is non-empty', () => {
    renderList(
      <EntryList
        local={[makeOrg(1, 'Local Org A')]}
        regional={[]}
        regionNameBySlug={emptyMap}
      />,
    );
    expect(screen.getByText('Local')).toBeDefined();
    expect(screen.queryByText('Regional')).toBeNull();
  });

  it('renders the Regional section label only when regional is non-empty', () => {
    renderList(
      <EntryList
        local={[]}
        regional={[makeOrg(2, 'Only Regional')]}
        regionNameBySlug={emptyMap}
      />,
    );
    expect(screen.queryByText('Local')).toBeNull();
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

  it('renders nothing when both sections are empty', () => {
    const { container } = renderList(
      <EntryList local={[]} regional={[]} regionNameBySlug={emptyMap} />,
    );
    // No section headers, no entries.
    expect(container.querySelector('.section-break')).toBeNull();
    expect(container.querySelector('.org-entry')).toBeNull();
  });

  it('shows the entry count in the section header', () => {
    renderList(
      <EntryList
        local={[makeOrg(1, 'A'), makeOrg(2, 'B'), makeOrg(3, 'C')]}
        regional={[makeOrg(4, 'D')]}
        regionNameBySlug={emptyMap}
      />,
    );
    expect(screen.getByText('3 entries')).toBeDefined();
    expect(screen.getByText('1 entry')).toBeDefined();
  });
});
