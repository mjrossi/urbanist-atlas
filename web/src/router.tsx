import { createBrowserRouter } from 'react-router';
import { App } from './App.tsx';
import { Home } from './routes/Home.tsx';

/**
 * The site's route tree. Today it's just the layout shell wrapping a
 * single placeholder Home route — real pages (slices #11–#16) plug
 * into this tree as additional children of the App route.
 *
 * `errorElement` is intentionally omitted for now; slice #15 adds a
 * shared not-found / error page.
 */
export const router = createBrowserRouter([
  {
    path: '/',
    Component: App,
    children: [{ index: true, Component: Home }],
  },
]);
