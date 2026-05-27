import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { RegionBreadcrumb } from './RegionBreadcrumb.tsx';
import type { Region } from '../lib/api.ts';

function makeRegion(slug: string, name: string): Region {
  return {
    id: 1,
    slug,
    name,
    country: 'US',
    kind: 'us:metro',
    scope_tier: 'regional',
    parent_slugs: [],
  };
}

describe('RegionBreadcrumb', () => {
  it('renders a Breadcrumb landmark with an ordered list', () => {
    render(
      <MemoryRouter>
        <RegionBreadcrumb
          prefix={[{ label: 'Atlas', to: '/' }, { label: 'Browse', to: '/browse' }]}
          ancestors={[makeRegion('washington-state', 'Washington')]}
          current="Seattle, WA"
        />
      </MemoryRouter>,
    );

    const nav = screen.getByRole('navigation', { name: /breadcrumb/i });
    expect(nav).toBeDefined();
    const list = within(nav).getByRole('list');
    expect(list.tagName).toBe('OL');
  });

  it('marks the trailing crumb with aria-current="page"', () => {
    render(
      <MemoryRouter>
        <RegionBreadcrumb
          prefix={[{ label: 'Atlas', to: '/' }]}
          ancestors={[]}
          current="Vancouver, BC"
        />
      </MemoryRouter>,
    );

    const here = screen.getByText('Vancouver, BC');
    expect(here.getAttribute('aria-current')).toBe('page');
  });

  it('renders prefix items without `to` as plain spans', () => {
    render(
      <MemoryRouter>
        <RegionBreadcrumb
          prefix={[
            { label: 'Atlas', to: '/' },
            { label: 'Lookup · 11217' },
          ]}
          ancestors={[]}
          current="Brooklyn, NY"
        />
      </MemoryRouter>,
    );

    // Atlas is a link
    expect(screen.getByRole('link', { name: 'Atlas' })).toBeDefined();
    // Lookup · 11217 is plain text (no link wrapper)
    expect(screen.queryByRole('link', { name: /lookup/i })).toBeNull();
    expect(screen.getByText(/lookup · 11217/i)).toBeDefined();
  });

  it('renders ancestors as Links to /region/:slug', () => {
    render(
      <MemoryRouter>
        <RegionBreadcrumb
          prefix={[]}
          ancestors={[makeRegion('washington-state', 'Washington')]}
          current="Seattle, WA"
        />
      </MemoryRouter>,
    );

    const wa = screen.getByRole('link', { name: 'Washington' });
    expect(wa.getAttribute('href')).toBe('/region/washington-state');
  });

  it('hides visual separators from screen readers', () => {
    const { container } = render(
      <MemoryRouter>
        <RegionBreadcrumb
          prefix={[{ label: 'Atlas', to: '/' }]}
          ancestors={[]}
          current="X"
        />
      </MemoryRouter>,
    );

    const seps = container.querySelectorAll('.crumb-sep');
    expect(seps.length).toBeGreaterThan(0);
    seps.forEach((sep) => {
      expect(sep.getAttribute('aria-hidden')).toBe('true');
    });
  });

  it('renders metaRight only when provided', () => {
    const { rerender } = render(
      <MemoryRouter>
        <RegionBreadcrumb
          prefix={[{ label: 'Atlas', to: '/' }]}
          ancestors={[]}
          current="Here"
          metaRight="extra context"
        />
      </MemoryRouter>,
    );
    expect(screen.getByText('extra context')).toBeDefined();

    rerender(
      <MemoryRouter>
        <RegionBreadcrumb
          prefix={[{ label: 'Atlas', to: '/' }]}
          ancestors={[]}
          current="Here"
        />
      </MemoryRouter>,
    );
    expect(screen.queryByText('extra context')).toBeNull();
  });
});
