import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { RouteObject } from 'react-router';
import { createBrowserRouter, createMemoryRouter, RouterProvider } from 'react-router';
import type { MockInstance } from 'vitest';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const pending = () => new Promise(() => {});

const { listRegionsMock } = vi.hoisted(() => ({ listRegionsMock: vi.fn() }));

vi.mock('./lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('./lib/api.ts')>('./lib/api.ts');
  return {
    ...actual,
    getStats: vi.fn(pending),
    listRegions: listRegionsMock,
    listRecent: vi.fn(pending),
  };
});

const { routes } = await import('./router.tsx');

// A router clears each `lazy` property on the route object once it has
// loaded it, so a second router built from the same objects would find
// nothing left to load. Each test router gets its own copy.
function cloneRoutes(list: RouteObject[]): RouteObject[] {
  return list.map((r) => ({
    ...r,
    ...(r.lazy && typeof r.lazy === 'object' ? { lazy: { ...r.lazy } } : {}),
    ...(r.children ? { children: cloneRoutes(r.children) } : {}),
  })) as RouteObject[];
}
const pristineRoutes = cloneRoutes(routes);

function mountAt(path: string) {
  const router = createMemoryRouter(cloneRoutes(pristineRoutes), {
    initialEntries: [path],
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

describe('routes', () => {
  beforeEach(() => {
    listRegionsMock.mockImplementation(pending);
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

// The browser router, the real layout and its ScrollRestoration, so a
// jump link goes through the same history entries and scroll keys as in
// production.
describe('in-page jump links', () => {
  let scrollY = 0;
  let scrollTo: MockInstance<Window['scrollTo']>;
  let scrolledIntoView: string[];
  let dispose: () => void = () => undefined;
  const nativeScrollIntoView = Object.getOwnPropertyDescriptor(
    Element.prototype,
    'scrollIntoView',
  );

  beforeEach(() => {
    scrollY = 0;
    scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation((_x, y) => {
      scrollY = typeof y === 'number' ? y : 0;
    });
    Object.defineProperty(window, 'scrollY', {
      configurable: true,
      get: () => scrollY,
    });
    scrolledIntoView = [];
    Element.prototype.scrollIntoView = function (this: Element) {
      scrolledIntoView.push(this.id);
    };
  });

  afterEach(() => {
    dispose();
    if (nativeScrollIntoView) {
      Object.defineProperty(Element.prototype, 'scrollIntoView', nativeScrollIntoView);
    } else {
      // @ts-expect-error jsdom has no scrollIntoView to put back.
      delete Element.prototype.scrollIntoView;
    }
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  function mountBrowserAt(path: string) {
    window.history.replaceState(null, '', path);
    const router = createBrowserRouter(cloneRoutes(pristineRoutes));
    dispose = () => {
      router.dispose();
    };
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
    return router;
  }

  async function jump(name: RegExp) {
    const link = await screen.findByRole('link', { name });
    // A reader partway down the page, as when they reach a jump strip.
    scrollY = 800;
    scrollTo.mockClear();
    await userEvent.click(link);
  }

  it('scrolls the About section into view instead of restoring the old offset', async () => {
    mountBrowserAt('/about');
    await jump(/II · Methodology/);
    expect(window.location.hash).toBe('#methodology');
    expect(scrolledIntoView).toEqual(['methodology']);
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it('scrolls a Browse letter row into view instead of restoring the old offset', async () => {
    listRegionsMock.mockResolvedValue([
      {
        region: {
          id: 1,
          kind: 'us:metro',
          name: 'Austin',
          slug: 'austin',
          country: 'US',
          scope_tier: 'regional',
          parent_slugs: [],
        },
        org_count: 1,
        direct_org_count: 1,
      },
    ]);
    mountBrowserAt('/browse?from=test');
    await jump(/^A$/);
    expect(window.location.pathname + window.location.search).toBe('/browse?from=test');
    expect(window.location.hash).toBe('#US-A');
    expect(scrolledIntoView).toEqual(['US-A']);
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it('leaves the skip link as a focus move, with no history entry or scroll', async () => {
    const router = mountBrowserAt('/about');
    await screen.findByRole('link', { name: /II · Methodology/ });
    await jump(/skip to content/i);
    expect(document.activeElement).toBe(screen.getByRole('main'));
    expect(router.state.location.hash).toBe('');
    expect(scrolledIntoView).toEqual([]);
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it('returns to the offset before the jump on Back', async () => {
    const router = mountBrowserAt('/about');
    await jump(/II · Methodology/);
    scrollY = 2400;
    await act(() => router.navigate(-1));
    expect(scrollTo).toHaveBeenLastCalledWith(0, 800);
  });
});
