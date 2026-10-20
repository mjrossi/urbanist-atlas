import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import {
  createBrowserRouter,
  createMemoryRouter,
  Outlet,
  RouterProvider,
} from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '../lib/api.ts';
import { lazyComponent } from '../lib/staleChunk.ts';
import { stubReload } from '../test/stubReload.ts';
import { RouteErrorBoundary } from './ErrorBoundary.tsx';

// A root layout with our errorElement (mirrors the production router):
// an unmatched path under '/' bubbles a 404 to the errorElement, and a
// child that throws routes its error there too. The masthead also renders
// an <h1>, so headings are queried by accessible name, not by level.
function renderAt(initialPath: string, boom?: ReactElement) {
  const router = createMemoryRouter(
    [
      {
        path: '/',
        element: <Outlet />,
        errorElement: <RouteErrorBoundary />,
        children: [
          { index: true, element: <div /> },
          { path: 'boom', element: boom ?? <div /> },
        ],
      },
    ],
    { initialEntries: [initialPath] },
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

function Boom({ error }: { error: unknown }): never {
  throw error;
}

describe('RouteErrorBoundary', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the 404 page for an unmatched route', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    renderAt('/no-such-page');
    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: /page not in this edition/i }),
      ).toBeDefined();
    });
  });

  it('renders the stop-press page and the request id for a thrown ApiError', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    renderAt(
      '/boom',
      <Boom error={new ApiError(500, 'boom', undefined, 'rid-abc123')} />,
    );
    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: /something went wrong/i }),
      ).toBeDefined();
    });
    expect(screen.getByText(/request id: rid-abc123/i)).toBeDefined();
  });

  it('renders the stop-press page without a request id for a plain error', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    renderAt('/boom', <Boom error={new Error('kaboom')} />);
    await waitFor(() => {
      expect(
        screen.getByRole('heading', { name: /something went wrong/i }),
      ).toBeDefined();
    });
    expect(screen.queryByText(/request id:/i)).toBeNull();
  });
});

describe('RouteErrorBoundary on a route chunk that fails to load', () => {
  const reload = vi.fn();
  let dispose: () => void = () => undefined;

  beforeEach(() => {
    reload.mockReset();
    stubReload(reload);
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined);
  });

  afterEach(() => {
    dispose();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  // The browser router, as in production, so the test sees which URL the
  // failed navigation leaves in the address bar.
  function mountStaleRoute(path = '/') {
    window.history.replaceState(null, '', path);
    const router = createBrowserRouter([
      {
        path: '/',
        element: <Outlet />,
        errorElement: <RouteErrorBoundary />,
        children: [
          { index: true, element: <div /> },
          {
            path: 'stale',
            lazy: lazyComponent(
              () =>
                Promise.reject(
                  new TypeError('Failed to fetch dynamically imported module'),
                ),
              () => () => null,
            ),
          },
        ],
      },
    ]);
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

  it('reloads the page it was navigating to, once', async () => {
    const router = mountStaleRoute();
    await act(() => router.navigate('/stale?q=1'));
    expect(router.state.location.pathname).toBe('/stale');
    expect(document.location.pathname + document.location.search).toBe('/stale?q=1');
    expect(reload).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('heading', { name: /new edition/i })).toBeNull();
  });

  it('reloads once when the first page view is the stale route', async () => {
    mountStaleRoute('/stale');
    await waitFor(() => {
      expect(reload).toHaveBeenCalledTimes(1);
    });
  });

  it('shows a reload link instead of reloading again', async () => {
    sessionStorage.setItem('atlas:chunk-reload', '1');
    const router = mountStaleRoute();
    await act(() => router.navigate('/stale?q=1'));
    expect(
      await screen.findByRole('heading', { name: /a new edition is out/i }),
    ).toBeDefined();
    expect(reload).not.toHaveBeenCalled();
    expect(
      screen.getByRole('link', { name: /reload the page/i }).getAttribute('href'),
    ).toBe('/stale?q=1');
  });

  it('shows the reload link when storage is unavailable to guard a loop', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });
    const router = mountStaleRoute();
    await act(() => router.navigate('/stale'));
    expect(
      await screen.findByRole('heading', { name: /a new edition is out/i }),
    ).toBeDefined();
    expect(reload).not.toHaveBeenCalled();
  });
});
