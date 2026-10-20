import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const pending = () => new Promise(() => {});

vi.mock('./lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('./lib/api.ts')>('./lib/api.ts');
  return {
    ...actual,
    getStats: vi.fn(pending),
    listRegions: vi.fn(pending),
    listRecent: vi.fn(pending),
  };
});

const { routes } = await import('./router.tsx');

function mountAt(path: string) {
  const router = createMemoryRouter(routes, { initialEntries: [path] });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

describe('routes', () => {
  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined);
  });

  it('bundles the front page and splits every other route', () => {
    const children = routes[0]?.children ?? [];
    const home = children.find((r) => r.index);
    expect(home?.Component).toBeDefined();
    expect(home?.lazy).toBeUndefined();
    const others = children.filter((r) => !r.index);
    expect(others.length).toBeGreaterThan(0);
    for (const r of others) {
      expect(r.lazy).toBeDefined();
      expect(r.Component).toBeUndefined();
    }
  });

  it('renders the front page', async () => {
    mountAt('/');
    expect(
      await screen.findByRole('heading', { name: /find local advocates/i }),
    ).toBeDefined();
  });

  it('loads a split route on first visit', async () => {
    mountAt('/about');
    expect(
      await screen.findByRole('heading', { level: 1, name: /doing the work/i }),
    ).toBeDefined();
  });

  it('still routes unknown paths to the 404 page', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    mountAt('/no-such-page');
    expect(
      await screen.findByRole('heading', { name: /not in this edition/i }),
    ).toBeDefined();
  });

  it('keeps the submissions desk reachable', async () => {
    mountAt('/submit');
    expect(await screen.findAllByRole('heading', { level: 1 })).not.toHaveLength(0);
  });
});
