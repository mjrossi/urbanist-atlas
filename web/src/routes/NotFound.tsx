import { Link } from 'react-router';
import { BroadsheetNav } from '../components/BroadsheetNav.tsx';
import { Footer } from '../components/Footer.tsx';
import { Masthead } from '../components/Masthead.tsx';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import { useScrollToTop } from '../lib/useScrollToTop.ts';

export function NotFound() {
  useDocumentTitle('Page not in this edition — Urbanist Atlas');
  return (
    <>
      <div className="kicker">
        <div>
          <Link to="/">Atlas</Link>
          <span className="crumb-sep">/</span>
          <span className="crumb-here">Not found</span>
        </div>
        <div>404 · Page retracted</div>
      </div>
      <div className="lede mt-56">
        <div className="eyebrow">
          § Retractions desk<span className="eyebrow-rule" />
        </div>
        <h1>
          Page <span className="accent">not in this edition.</span>
        </h1>
        <p className="deck">
          The story you were looking for could not be found in our directory.
          The link you followed either never went to press, or has since been
          retired.
        </p>
      </div>
      <div className="spread mt-24">
        <div className="prose">
          <p>
            If you followed a link from another site and you think this page
            should still exist, please{' '}
            <Link to="/submit">file a tip at the submissions desk</Link> with
            the URL you tried — we&rsquo;ll either restore the entry or update
            the redirect.
          </p>
          <p>
            Otherwise, the front page is the best place to start:
          </p>
          <p>
            <Link to="/" className="btn-primary not-found-return">
              Return to the front page <span className="arrow">→</span>
            </Link>
          </p>
        </div>
        <aside className="rail">
          <div className="rail-block">
            <div className="rail-kicker">Common destinations</div>
            <ul className="plain">
              <li>
                <Link to="/">Front page · the lookup</Link>
              </li>
              <li>
                <Link to="/browse">Browse · the index</Link>
              </li>
              <li>
                <Link to="/about">About the Atlas</Link>
              </li>
              <li>
                <Link to="/submit">Submissions desk</Link>
              </li>
            </ul>
          </div>
        </aside>
      </div>
    </>
  );
}

export function NotFoundWithLayout() {
  useScrollToTop();
  return (
    <div className="sheet">
      <Masthead />
      <BroadsheetNav />
      <main>
        <NotFound />
      </main>
      <Footer />
    </div>
  );
}
