import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { PageBreadcrumb } from './PageBreadcrumb.tsx';

describe('PageBreadcrumb', () => {
  it('renders a Breadcrumb landmark with an ordered list', () => {
    render(
      <MemoryRouter>
        <PageBreadcrumb prefix={[{ label: 'Atlas', to: '/' }]} current="Submissions" />
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
        <PageBreadcrumb prefix={[{ label: 'Atlas', to: '/' }]} current="About" />
      </MemoryRouter>,
    );

    const here = screen.getByText('About');
    expect(here.getAttribute('aria-current')).toBe('page');
  });

  it('renders prefix items with `to` as Links pointing at the right href', () => {
    render(
      <MemoryRouter>
        <PageBreadcrumb
          prefix={[
            { label: 'Atlas', to: '/' },
            { label: 'Browse', to: '/browse' },
          ]}
          current="Transportation Alternatives"
        />
      </MemoryRouter>,
    );

    expect(screen.getByRole('link', { name: 'Atlas' }).getAttribute('href')).toBe('/');
    expect(screen.getByRole('link', { name: 'Browse' }).getAttribute('href')).toBe('/browse');
  });

  it('renders prefix items without `to` as plain text (no link wrapper)', () => {
    render(
      <MemoryRouter>
        <PageBreadcrumb
          prefix={[{ label: 'The front page' }]}
          current="Index by postal code"
        />
      </MemoryRouter>,
    );

    expect(screen.queryByRole('link', { name: /front page/i })).toBeNull();
    expect(screen.getByText('The front page')).toBeDefined();
  });

  it('hides visual separators from screen readers', () => {
    const { container } = render(
      <MemoryRouter>
        <PageBreadcrumb prefix={[{ label: 'Atlas', to: '/' }]} current="X" />
      </MemoryRouter>,
    );
    const seps = container.querySelectorAll('.crumb-sep');
    expect(seps.length).toBeGreaterThan(0);
    seps.forEach((sep) => {
      expect(sep.getAttribute('aria-hidden')).toBe('true');
    });
  });

  it('renders meta only when provided', () => {
    const { rerender } = render(
      <MemoryRouter>
        <PageBreadcrumb
          prefix={[{ label: 'Atlas', to: '/' }]}
          current="Colophon"
          meta="Volume I · 2026 Edition"
        />
      </MemoryRouter>,
    );
    expect(screen.getByText('Volume I · 2026 Edition')).toBeDefined();

    rerender(
      <MemoryRouter>
        <PageBreadcrumb prefix={[{ label: 'Atlas', to: '/' }]} current="Colophon" />
      </MemoryRouter>,
    );
    expect(screen.queryByText('Volume I · 2026 Edition')).toBeNull();
  });

  it('renders a single-leaf breadcrumb when prefix is omitted', () => {
    render(
      <MemoryRouter>
        <PageBreadcrumb current="The index" />
      </MemoryRouter>,
    );
    const here = screen.getByText('The index');
    expect(here.getAttribute('aria-current')).toBe('page');
    expect(screen.queryByRole('link')).toBeNull();
  });
});
