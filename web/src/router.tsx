import type { RouteObject } from 'react-router';
import { createBrowserRouter } from 'react-router';

import { App } from './App.tsx';
import { SheetLayout } from './components/SheetLayout.tsx';
import { lazyComponent } from './lib/staleChunk.ts';
import { RouteErrorBoundary } from './routes/ErrorBoundary.tsx';
import { Home } from './routes/Home.tsx';

/**
 * The site's route tree. The front page ships in the main bundle; every
 * other route is a lazy chunk. `errorElement` on the root catches
 * unmatched URLs (404), errors thrown by a route, and a chunk that
 * fails to load (see `lazyComponent`); it renders instead of `App` and
 * wraps its own chrome.
 */
export const routes: RouteObject[] = [
  {
    path: '/',
    Component: App,
    errorElement: <RouteErrorBoundary />,
    // Shown while a lazy route's chunk loads on the first page view.
    hydrateFallbackElement: <SheetLayout>{null}</SheetLayout>,
    children: [
      { index: true, Component: Home },
      {
        path: 'r/:postalCode',
        lazy: lazyComponent(
          () => import('./routes/Results.tsx'),
          (m) => m.Results,
        ),
      },
      {
        path: 'browse',
        lazy: lazyComponent(
          () => import('./routes/Browse.tsx'),
          (m) => m.Browse,
        ),
      },
      {
        path: 'region/:regionSlug',
        lazy: lazyComponent(
          () => import('./routes/Region.tsx'),
          (m) => m.Region,
        ),
      },
      {
        path: 'orgs/:slug',
        lazy: lazyComponent(
          () => import('./routes/Org.tsx'),
          (m) => m.Org,
        ),
      },
      {
        path: 'submit',
        lazy: lazyComponent(
          () => import('./routes/Submit.tsx'),
          (m) => m.Submit,
        ),
      },
      {
        path: 'about',
        lazy: lazyComponent(
          () => import('./routes/About.tsx'),
          (m) => m.About,
        ),
      },
      {
        path: 'colophon',
        lazy: lazyComponent(
          () => import('./routes/Colophon.tsx'),
          (m) => m.Colophon,
        ),
      },
    ],
  },
];

export const router = createBrowserRouter(routes);
