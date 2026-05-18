import { createBrowserRouter } from 'react-router';
import { App } from './App.tsx';
import { Browse } from './routes/Browse.tsx';
import { Home } from './routes/Home.tsx';
import { Metro } from './routes/Metro.tsx';
import { Results } from './routes/Results.tsx';

/**
 * The site's route tree. Home and Results landed in slices #11 + #12;
 * Browse and Metro land in slice #14. Submit + about + a shared 404
 * follow in slices #15+.
 *
 * `errorElement` is intentionally omitted for now; slice #15 adds a
 * shared not-found / error page.
 */
export const router = createBrowserRouter([
  {
    path: '/',
    Component: App,
    children: [
      { index: true, Component: Home },
      { path: 'r/:postalCode', Component: Results },
      { path: 'browse', Component: Browse },
      { path: 'm/:metroSlug', Component: Metro },
    ],
  },
]);
