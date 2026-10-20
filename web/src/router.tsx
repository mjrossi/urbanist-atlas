import type { RouteObject } from 'react-router';
import { createBrowserRouter } from 'react-router';

import { App } from './App.tsx';
import { SheetLayout } from './components/SheetLayout.tsx';
import { RouteErrorBoundary } from './routes/ErrorBoundary.tsx';
import { Home } from './routes/Home.tsx';

/**
 * The site's route tree. The front page ships in the main bundle; every
 * other route is a lazy chunk. `errorElement` on the root catches
 * unmatched URLs (404), errors thrown by a route, and a chunk that
 * fails to load; it renders instead of `App` and wraps its own chrome.
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
        lazy: { Component: async () => (await import('./routes/Results.tsx')).Results },
      },
      {
        path: 'browse',
        lazy: { Component: async () => (await import('./routes/Browse.tsx')).Browse },
      },
      {
        path: 'region/:regionSlug',
        lazy: { Component: async () => (await import('./routes/Region.tsx')).Region },
      },
      {
        path: 'orgs/:slug',
        lazy: { Component: async () => (await import('./routes/Org.tsx')).Org },
      },
      {
        path: 'submit',
        lazy: { Component: async () => (await import('./routes/Submit.tsx')).Submit },
      },
      {
        path: 'about',
        lazy: { Component: async () => (await import('./routes/About.tsx')).About },
      },
      {
        path: 'colophon',
        lazy: {
          Component: async () => (await import('./routes/Colophon.tsx')).Colophon,
        },
      },
    ],
  },
];

export const router = createBrowserRouter(routes);
