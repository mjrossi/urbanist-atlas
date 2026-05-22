import { Link } from 'react-router';
import { BroadsheetNav } from '../components/BroadsheetNav.tsx';
import { Footer } from '../components/Footer.tsx';
import { Masthead } from '../components/Masthead.tsx';

/**
 * Newspaper-style 404. Wired as `errorElement` on the root route in
 * `router.tsx`, which means it renders for any unmatched URL **and**
 * for unhandled errors thrown anywhere in the route tree.
 *
 * Reuses the `.page` single-column treatment so the body matches the
 * other inner pages (About, Browse, Metro, Results). The chrome
 * (masthead/nav/footer) is added by {@link NotFoundWithLayout}, which
 * is what the router actually mounts — `errorElement` renders
 * **instead of** the root route's `App` component, so the layout has
 * to be reconstructed.
 */
export function NotFound() {
  return (
    <div className="page">
      <header className="page-header">
        <h1>Page not in this edition.</h1>
        <p>
          <em>
            The story you were looking for could not be found in our directory.
          </em>
        </p>
      </header>
      <section>
        <p>
          We’ve pulled this page from the edition, with apologies. The link
          you followed either never went to press, or has since been retired.
        </p>
        <p>
          <Link to="/" className="not-found-return">
            Return to the front page
          </Link>
        </p>
      </section>
    </div>
  );
}

/**
 * Chrome-wrapped {@link NotFound}, suitable for use as the root route's
 * `errorElement` in {@link ../router.tsx}. Keeps the masthead, nav,
 * and footer visible on the 404 surface so it still feels like the
 * Atlas, not a bare browser error page.
 */
export function NotFoundWithLayout() {
  return (
    <>
      <Masthead />
      <BroadsheetNav />
      <main>
        <NotFound />
      </main>
      <Footer />
    </>
  );
}
