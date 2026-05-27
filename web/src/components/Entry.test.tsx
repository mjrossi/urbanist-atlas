import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { Entry } from './Entry.tsx';
import type { Org } from '../lib/api.ts';

function makeOrg(overrides: Partial<Org> = {}): Org {
  return {
    id: 1,
    slug: 'transalt',
    name: 'Transportation Alternatives',
    short_desc: 'NYC-wide advocacy for walking, biking, and public transit.',
    website_url: 'https://www.transalt.org',
    tags: ['transit', 'safe-streets'],
    regions: [],
    ...overrides,
  };
}

/**
 * `<Entry>` uses `<Link>` for the org name, so every test renders
 * inside a `MemoryRouter` so the router context resolves.
 */
function renderEntry(node: React.ReactNode) {
  return render(<MemoryRouter>{node}</MemoryRouter>);
}

describe('Entry', () => {
  it('renders name as an internal link to /orgs/:slug', () => {
    renderEntry(<Entry org={makeOrg()} />);
    const link = screen.getByRole('link', { name: 'Transportation Alternatives' });
    expect(link.getAttribute('href')).toBe('/orgs/transalt');
  });

  it('renders the website domain as a secondary external link', () => {
    renderEntry(<Entry org={makeOrg()} />);
    const link = screen.getByRole('link', { name: 'transalt.org' });
    expect(link.getAttribute('href')).toBe('https://www.transalt.org');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toContain('noopener');
    expect(link.getAttribute('rel')).toContain('noreferrer');
  });

  it('strips the leading www. when displaying the domain', () => {
    renderEntry(<Entry org={makeOrg()} />);
    expect(screen.getByText('transalt.org')).toBeDefined();
  });

  it('renders each tag in the .tag-list', () => {
    renderEntry(
      <Entry
        org={makeOrg({ tags: ['cycling', 'policy', 'grassroots'] })}
      />,
    );
    expect(screen.getByText('cycling')).toBeDefined();
    expect(screen.getByText('policy')).toBeDefined();
    expect(screen.getByText('grassroots')).toBeDefined();
  });

  it('omits the tags list when the org has no tags', () => {
    const { container } = renderEntry(<Entry org={makeOrg({ tags: [] })} />);
    expect(container.querySelector('.tag-list')).toBeNull();
  });

  it('falls back to the raw URL as link text when website_url is malformed', () => {
    // Admin-curated data: render the outbound affordance even if
    // domainOf can't extract a hostname. A visibly-broken link is
    // more useful to the user (and to us) than a silently-missing
    // one.
    renderEntry(<Entry org={makeOrg({ website_url: 'not a url' })} />);
    const link = screen.getByRole('link', { name: 'not a url' });
    expect(link.getAttribute('href')).toBe('not a url');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toContain('noopener');
  });

  it('renders "Matched via X" footer when matchedRegionSlugs is non-empty', () => {
    const slugMap = new Map([['brooklyn', 'Brooklyn']]);
    renderEntry(
      <Entry
        org={makeOrg()}
        matchedRegionSlugs={['brooklyn']}
        regionNameBySlug={slugMap}
      />,
    );
    expect(screen.getByText(/Matched via/)).toBeDefined();
    expect(screen.getByText('Brooklyn')).toBeDefined();
  });

  it('falls back to the raw slug when no display name is mapped', () => {
    renderEntry(
      <Entry
        org={makeOrg()}
        matchedRegionSlugs={['some-unknown-slug']}
        regionNameBySlug={new Map()}
      />,
    );
    expect(screen.getByText('some-unknown-slug')).toBeDefined();
  });

  it('omits the Matched-via footer when matchedRegionSlugs is undefined', () => {
    const { container } = renderEntry(<Entry org={makeOrg()} />);
    expect(container.querySelector('.foot')).toBeNull();
  });

  it('omits the Matched-via footer when matchedRegionSlugs is empty', () => {
    const { container } = renderEntry(
      <Entry org={makeOrg()} matchedRegionSlugs={[]} />,
    );
    expect(container.querySelector('.foot')).toBeNull();
  });

  it('percent-encodes the slug in the detail link', () => {
    renderEntry(
      <Entry
        org={makeOrg({ slug: 'weird slug', name: 'Weird Org' })}
      />,
    );
    const link = screen.getByRole('link', { name: 'Weird Org' });
    expect(link.getAttribute('href')).toBe('/orgs/weird%20slug');
  });
});
