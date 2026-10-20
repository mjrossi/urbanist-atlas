import { render, screen } from '@testing-library/react';
import type { RouteObject } from 'react-router';
import { createMemoryRouter, Outlet, RouterProvider } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { stubReload } from '../test/stubReload.ts';
import {
  canReloadForChunk,
  ChunkLoadError,
  lazyComponent,
  reloadForChunk,
} from './staleChunk.ts';

const FLAG = 'atlas:chunk-reload';

function Page() {
  return <h1>Loaded</h1>;
}

describe('lazyComponent', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it('tags a rejected import as a ChunkLoadError carrying the cause', async () => {
    const cause = new TypeError('Failed to fetch dynamically imported module');
    const load = lazyComponent(
      () => Promise.reject(cause),
      () => Page,
    );
    const err: unknown = await load().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ChunkLoadError);
    expect((err as ChunkLoadError).cause).toBe(cause);
  });

  it('clears the reload flag once a chunk loads', async () => {
    sessionStorage.setItem(FLAG, '1');
    const load = lazyComponent(
      () => Promise.resolve({ Page }),
      (m) => m.Page,
    );
    await expect(load()).resolves.toEqual({ Component: Page });
    expect(sessionStorage.getItem(FLAG)).toBeNull();
  });

  it('keeps the flag when the import fails again', async () => {
    sessionStorage.setItem(FLAG, '1');
    const load = lazyComponent(
      () => Promise.reject(new Error('gone')),
      () => Page,
    );
    await load().catch(() => undefined);
    expect(sessionStorage.getItem(FLAG)).toBe('1');
  });

  // Why lazyComponent returns the function form of `lazy`.
  describe('how React Router surfaces a rejected lazy load', () => {
    function mount(child: RouteObject) {
      vi.spyOn(console, 'error').mockImplementation(() => undefined);
      vi.spyOn(console, 'warn').mockImplementation(() => undefined);
      const router = createMemoryRouter(
        [
          {
            path: '/',
            element: <Outlet />,
            errorElement: <p>boundary</p>,
            children: [{ index: true, element: <p>home</p> }, child],
          },
        ],
        { initialEntries: ['/'] },
      );
      render(<RouterProvider router={router} />);
      return router;
    }
    const reject = () => Promise.reject(new Error('gone'));

    it('renders an empty route for a rejected lazy Component property', async () => {
      const router = mount({ path: 'x', lazy: { Component: reject } });
      await router.navigate('/x');
      expect(router.state.errors).toBeNull();
      expect(screen.queryByText('boundary')).toBeNull();
    });

    it('reaches the error boundary for a rejected lazy function', async () => {
      const router = mount({
        path: 'x',
        lazy: lazyComponent(reject, () => Page),
      });
      await router.navigate('/x');
      expect(await screen.findByText('boundary')).toBeDefined();
    });
  });
});

describe('reloadForChunk', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it('reloads once, then refuses', () => {
    const reload = vi.fn();
    stubReload(reload);
    expect(canReloadForChunk()).toBe(true);
    reloadForChunk();
    reloadForChunk();
    expect(reload).toHaveBeenCalledTimes(1);
    expect(canReloadForChunk()).toBe(false);
  });

  it('never reloads when storage throws', () => {
    const reload = vi.fn();
    stubReload(reload);
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('full', 'QuotaExceededError');
    });
    reloadForChunk();
    expect(reload).not.toHaveBeenCalled();
    expect(canReloadForChunk()).toBe(true);
  });
});
