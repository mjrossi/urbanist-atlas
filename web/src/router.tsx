import { createBrowserRouter } from 'react-router';
import { App } from './App.tsx';
import { About } from './routes/About.tsx';
import { Browse } from './routes/Browse.tsx';
import { Colophon } from './routes/Colophon.tsx';
import { Home } from './routes/Home.tsx';
import { Metro } from './routes/Metro.tsx';
import { NotFoundWithLayout } from './routes/NotFound.tsx';
import { Org } from './routes/Org.tsx';
import { Results } from './routes/Results.tsx';
import { Submit } from './routes/Submit.tsx';

/**
 * The site's route tree. Home and Results landed in slices #11 + #12;
 * Browse and Metro in slice #14; About and the newspaper-style 404
 * land in slice #15. /submit is a Phase 2 placeholder — the nav
 * advertises it, so it needs a real "coming with Phase 2" page rather
 * than falling through to the 404 errorElement.
 *
 * `errorElement` on the root route catches both unmatched URLs (404)
 * and any unhandled error thrown by a descendant route's component
 * or loader. It renders **instead of** `App`, so the chrome-wrapping
 * lives inside `NotFoundWithLayout` (see `routes/NotFound.tsx`).
 */
export const router = createBrowserRouter([
  {
    path: '/',
    Component: App,
    errorElement: <NotFoundWithLayout />,
    children: [
      { index: true, Component: Home },
      { path: 'r/:postalCode', Component: Results },
      { path: 'browse', Component: Browse },
      { path: 'm/:metroSlug', Component: Metro },
      { path: 'orgs/:slug', Component: Org },
      { path: 'submit', Component: Submit },
      { path: 'about', Component: About },
      { path: 'colophon', Component: Colophon },
    ],
  },
]);
