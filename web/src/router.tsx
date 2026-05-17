import { createBrowserRouter } from 'react-router';
import { App } from './App.tsx';
import { Home } from './routes/Home.tsx';
import { Results } from './routes/Results.tsx';

/**
 * The site's route tree. Home and Results land in slices #11 + #12;
 * remaining pages (browse, submit, about, 404) plug in as additional
 * children of the App route in slices #13–#15.
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
    ],
  },
]);
