import type { RouteObject } from 'react-router';
import { createBrowserRouter } from 'react-router';

import { App } from './App.tsx';
import { SheetLayout } from './components/SheetLayout.tsx';
import { lazyComponent } from './lib/staleChunk.ts';
import { RouteErrorBoundary } from './routes/ErrorBoundary.tsx';
import { Home } from './routes/Home.tsx';

/**
 * The route tree. Every route but the front page is a lazy chunk. The
 * root `errorElement` replaces `App` for an unmatched URL, a thrown
 * error or a failed chunk load, so it renders its own chrome.
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
