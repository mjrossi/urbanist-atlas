import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Entry } from './Entry.tsx';
import type { LookupOrg } from '../lib/api.ts';

function makeOrg(overrides: Partial<LookupOrg> = {}): LookupOrg {
  return {
    id: 1,
    slug: 'transalt',
    name: 'Transportation Alternatives',
    short_desc: 'NYC-wide advocacy for walking, biking, and public transit.',
    website_url: 'https://www.transalt.org',
    tags: ['transit', 'safe-streets'],
    regions: [],
    matched_region_slugs: [],
    ...overrides,
  };
}

const emptyMap = new Map<string, string>();

describe('Entry', () => {
  it('renders name as an external link with rel=noopener', () => {
    render(
      <ul>
        <Entry org={makeOrg()} regionNameBySlug={emptyMap} />
      </ul>,
    );
    const link = screen.getByRole('link', { name: 'Transportation Alternatives' });
    expect(link.getAttribute('href')).toBe('https://www.transalt.org');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toContain('noopener');
  });

  it('strips the leading www. when displaying the domain', () => {
    render(
      <ul>
        <Entry org={makeOrg()} regionNameBySlug={emptyMap} />
      </ul>,
    );
    expect(screen.getByText('transalt.org')).toBeDefined();
  });

  it('renders each tag as a TagChip', () => {
    render(
      <ul>
        <Entry
          org={makeOrg({ tags: ['cycling', 'policy', 'grassroots'] })}
          regionNameBySlug={emptyMap}
        />
      </ul>,
    );
    expect(screen.getByText('cycling')).toBeDefined();
    expect(screen.getByText('policy')).toBeDefined();
    expect(screen.getByText('grassroots')).toBeDefined();
  });

  it('omits the tags list when the org has no tags', () => {
    const { container } = render(
      <ul>
        <Entry org={makeOrg({ tags: [] })} regionNameBySlug={emptyMap} />
      </ul>,
    );
    expect(container.querySelector('.entry-tags')).toBeNull();
  });

  it('gracefully drops the domain hint when website_url is malformed', () => {
    const { container } = render(
      <ul>
        <Entry org={makeOrg({ website_url: 'not a url' })} regionNameBySlug={emptyMap} />
      </ul>,
    );
    expect(container.querySelector('.entry-domain')).toBeNull();
  });

  it('renders "via X" subtitle when matched_region_slugs is non-empty', () => {
    const slugMap = new Map([['brooklyn', 'Brooklyn']]);
    render(
      <ul>
        <Entry org={makeOrg({ matched_region_slugs: ['brooklyn'] })} regionNameBySlug={slugMap} />
      </ul>,
    );
    expect(screen.getByText('via Brooklyn')).toBeDefined();
  });

  it('omits the "via" subtitle when matched_region_slugs is empty', () => {
    const { container } = render(
      <ul>
        <Entry org={makeOrg({ matched_region_slugs: [] })} regionNameBySlug={emptyMap} />
      </ul>,
    );
    expect(container.querySelector('.entry-via')).toBeNull();
  });
});
