import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { createMemoryRouter, Link, Outlet, RouterProvider } from 'react-router';
import type { MockInstance } from 'vitest';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { getStatsMock } = vi.hoisted(() => ({ getStatsMock: vi.fn() }));

vi.mock('../lib/api.ts', async () => {
  const actual = await vi.importActual<typeof import('../lib/api.ts')>('../lib/api.ts');
  return { ...actual, getStats: getStatsMock };
});

const { SheetLayout } = await import('./SheetLayout.tsx');

function Layout() {
  return (
    <SheetLayout>
      <Outlet />
    </SheetLayout>
  );
}

function PageA() {
  return (
    <>
      <h1>Page A</h1>
      <Link to="/b">to b</Link>
    </>
  );
}

function PageB() {
  return <h1>Page B</h1>;
}

function LateHeading() {
  const [ready, setReady] = useState(false);
  return (
    <>
      <button
        type="button"
        onClick={() => {
          setReady(true);
        }}
      >
        resolve
      </button>
      {ready ? <h1>Late heading</h1> : <p>Loading…</p>}
    </>
  );
}

function mountAt(path: string) {
  const router = createMemoryRouter(
    [
      {
        path: '/',
        Component: Layout,
        children: [
          { path: 'a', Component: PageA },
          { path: 'b', Component: PageB },
          { path: 'late', Component: LateHeading },
        ],
      },
    ],
    { initialEntries: [path] },
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

describe('SheetLayout', () => {
  let scrollY = 0;
  let scrollTo: MockInstance<Window['scrollTo']>;

  beforeEach(() => {
    getStatsMock.mockReturnValue(new Promise(() => {}));
    scrollY = 0;
    scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation((_x, y) => {
      scrollY = typeof y === 'number' ? y : 0;
    });
    Object.defineProperty(window, 'scrollY', {
      configurable: true,
      get: () => scrollY,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it('renders exactly one main landmark', () => {
    mountAt('/a');
    expect(screen.getAllByRole('main')).toHaveLength(1);
  });

  it('puts a skip link to the main content before the masthead', () => {
    mountAt('/a');
    const skip = screen.getByRole('link', { name: 'Skip to content' });
    const main = screen.getByRole('main');
    expect(skip.getAttribute('href')).toBe(`#${main.id}`);
    expect(document.querySelector('a')).toBe(skip);
  });

  it('moves focus to the main content when the skip link is used', async () => {
    const user = userEvent.setup();
    mountAt('/a');
    await user.click(screen.getByRole('link', { name: 'Skip to content' }));
    expect(document.activeElement).toBe(screen.getByRole('main'));
  });

  it('leaves focus alone on the first load', () => {
    mountAt('/a');
    expect(document.activeElement).toBe(document.body);
  });

  it('focuses the new page heading after a client-side navigation', async () => {
    const user = userEvent.setup();
    mountAt('/a');
    await user.click(screen.getByRole('link', { name: 'to b' }));
    const heading = await screen.findByRole('heading', { name: 'Page B' });
    await waitFor(() => {
      expect(document.activeElement).toBe(heading);
    });
    expect(heading.getAttribute('tabindex')).toBe('-1');
  });

  it('hands focus to a heading that renders after the navigation', async () => {
    const router = mountAt('/a');
    await act(() => router.navigate('/late'));
    expect(document.activeElement).toBe(screen.getByRole('main'));
    act(() => {
      // A programmatic click, so focus stays on main as it would while a
      // query resolves.
      screen.getByRole('button', { name: 'resolve' }).click();
    });
    const heading = await screen.findByRole('heading', { name: 'Late heading' });
    await waitFor(() => {
      expect(document.activeElement).toBe(heading);
    });
  });

  it('does not pull focus back once the reader has moved it', async () => {
    const user = userEvent.setup();
    const router = mountAt('/a');
    await act(() => router.navigate('/late'));
    const button = screen.getByRole('button', { name: 'resolve' });
    await user.click(button);
    await screen.findByRole('heading', { name: 'Late heading' });
    expect(document.activeElement).toBe(button);
  });

  it('starts a new page at the top and restores the old offset on Back', async () => {
    const router = mountAt('/a');
    scrollY = 640;
    await act(() => router.navigate('/b'));
    expect(scrollTo).toHaveBeenLastCalledWith(0, 0);
    await act(() => router.navigate(-1));
    expect(scrollTo).toHaveBeenLastCalledWith(0, 640);
  });
});
