import { Link } from 'react-router';

import { PadThaiIllustration } from '../components/NewsroomCats.tsx';
import { PageBreadcrumb } from '../components/PageBreadcrumb.tsx';
import { SheetLayout } from '../components/SheetLayout.tsx';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

export function NotFound() {
  useDocumentTitle('Page not in this edition — Urbanist Atlas');
  return (
    <>
      <PageBreadcrumb
        prefix={[{ label: 'Atlas', to: '/' }]}
        current="Not found"
        meta="404 · Page retracted"
      />
      <div className="lede mt-56">
        <div className="eyebrow">
          § Retractions desk
          <span className="eyebrow-rule" />
        </div>
        <h1>
          Page <span className="accent">not in this edition.</span>
        </h1>
        <p className="deck">
          We couldn&rsquo;t find the page you were after. The link you followed either
          never went to press, or has since been retired.
        </p>
      </div>
      <div className="spread mt-24">
        <div className="prose">
          <p>
            If you followed a link from another site and you think this page should still
            exist, please <Link to="/submit">file a tip at the submissions desk</Link>{' '}
            with the URL you tried — we&rsquo;ll either restore the entry or update the
            redirect.
          </p>
          <p>Otherwise, the front page is the best place to start:</p>
          <figure className="not-found-cat">
            <PadThaiIllustration className="cat-illustration" />
            <figcaption>
              The newsroom cat was dispatched to find the missing page. He found a patch
              of sun instead. The investigation is closed.
            </figcaption>
          </figure>
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
  return (
    <SheetLayout>
      <NotFound />
    </SheetLayout>
  );
}
