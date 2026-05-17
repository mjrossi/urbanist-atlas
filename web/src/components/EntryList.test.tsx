import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { EntryList } from './EntryList.tsx';
import type { Org } from '../lib/api.ts';

function makeOrg(id: number, name: string, tags: string[] = []): Org {
  return {
    id,
    slug: `org-${id}`,
    name,
    short_desc: `${name} short description`,
    website_url: `https://example.com/${id}`,
    tags,
    regions: [],
  };
}

describe('EntryList', () => {
  it('renders both Local and Regional section labels', () => {
    render(<EntryList local={[]} regional={[]} />);
    expect(screen.getByText('Local')).toBeDefined();
    expect(screen.getByText('Regional')).toBeDefined();
  });

  it('renders entries inside the matching section', () => {
    render(
      <EntryList
        local={[makeOrg(1, 'Local Org A'), makeOrg(2, 'Local Org B')]}
        regional={[makeOrg(3, 'Regional Org C')]}
      />,
    );
    expect(screen.getByRole('link', { name: 'Local Org A' })).toBeDefined();
    expect(screen.getByRole('link', { name: 'Local Org B' })).toBeDefined();
    expect(screen.getByRole('link', { name: 'Regional Org C' })).toBeDefined();
  });

  it('shows a per-section empty hint when a tier is empty', () => {
    render(<EntryList local={[]} regional={[makeOrg(1, 'Only Regional')]} />);
    expect(screen.getByText('No local groups indexed yet.')).toBeDefined();
    expect(screen.queryByText('No regional groups indexed yet.')).toBeNull();
  });
});
