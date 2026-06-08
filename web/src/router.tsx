import { createBrowserRouter } from 'react-router';

import { App } from './App.tsx';
import { About } from './routes/About.tsx';
import { Browse } from './routes/Browse.tsx';
import { Colophon } from './routes/Colophon.tsx';
import { RouteErrorBoundary } from './routes/ErrorBoundary.tsx';
import { Home } from './routes/Home.tsx';
import { Org } from './routes/Org.tsx';
import { Region } from './routes/Region.tsx';
import { Results } from './routes/Results.tsx';
import { Submit } from './routes/Submit.tsx';

/**
 * The site's route tree. Home and Results landed in slices #11 + #12;
 * Browse and Region in slice #14; About and the newspaper-style 404
 * land in slice #15. /submit is a Phase 2 placeholder — the nav
 * advertises it, so it needs a real "coming with Phase 2" page rather
 * than falling through to the 404 errorElement.
 *
 * `errorElement` on the root route catches both unmatched URLs (404)
 * and any unhandled error thrown by a descendant route's component
 * or loader. `RouteErrorBoundary` (see `routes/ErrorBoundary.tsx`)
 * branches: 404s keep the newspaper "not in this edition" page;
 * genuine errors render a distinct "stop press" page that surfaces the
 * request id for log correlation. Both render **instead of** `App` and
 * wrap their own chrome.
 */
export const router = createBrowserRouter([
  {
    path: '/',
    Component: App,
    errorElement: <RouteErrorBoundary />,
    children: [
      { index: true, Component: Home },
      { path: 'r/:postalCode', Component: Results },
      { path: 'browse', Component: Browse },
      { path: 'region/:regionSlug', Component: Region },
      { path: 'orgs/:slug', Component: Org },
      { path: 'submit', Component: Submit },
      { path: 'about', Component: About },
      { path: 'colophon', Component: Colophon },
    ],
  },
]);
