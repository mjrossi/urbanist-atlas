import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { createMemoryRouter, Outlet, RouterProvider } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '../lib/api.ts';
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
