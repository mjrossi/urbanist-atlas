import { useEffect } from 'react';
import { isRouteErrorResponse, Link, useRouteError } from 'react-router';

import { PageBreadcrumb } from '../components/PageBreadcrumb.tsx';
import { SheetLayout } from '../components/SheetLayout.tsx';
import { reportClientError, requestIdOf } from '../lib/clientErrors.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import { NotFoundWithLayout } from './NotFound.tsx';

/**
 * The root `errorElement`. React Router routes BOTH unmatched URLs (a
 * 404 ErrorResponse) and any unhandled error thrown by a descendant
 * component or loader here. We branch on which it is: a 404 keeps the
 * newspaper "page not in this edition" page; anything else is an
 * unexpected failure and gets its own "stop press" page rather than
 * masquerading as a 404 (the previous behavior).
 *
 * There is no error-tracking SaaS by design (see the observability
 * design spec). The boundary's whole job is to fail gracefully, surface
 * the request id so a user can quote it in a report, and log to the
 * console in dev — the maintainer then greps the server logs by that id.
 */
export function RouteErrorBoundary() {
  const error = useRouteError();
  if (isRouteErrorResponse(error) && error.status === 404) {
    return <NotFoundWithLayout />;
  }
  return <InternalErrorWithLayout error={error} />;
}

function InternalErrorWithLayout({ error }: { error: unknown }) {
  useDocumentTitle('Something went wrong — Urbanist Atlas');
  useEffect(() => {
    reportClientError('route error', error);
  }, [error]);

  const requestId = requestIdOf(error);
  return (
    <SheetLayout>
      <PageBreadcrumb
        prefix={[{ label: 'Atlas', to: '/' }]}
        current="Error"
        meta="500 · Stop press"
      />
      <div className="lede mt-56">
        <div className="eyebrow">
          § Stop press
          <span className="eyebrow-rule" />
        </div>
        <h1>
          Something <span className="accent">went wrong.</span>
        </h1>
        <p className="deck">
          An unexpected error interrupted this page. It&rsquo;s on our end, not yours —
          please try again in a moment.
        </p>
      </div>
      <div className="spread mt-24">
        <div className="prose">
          <p>
            If it keeps happening, please{' '}
            <Link to="/submit">file a tip at the submissions desk</Link>
            {requestId
              ? ' and include the reference below so we can trace it in our logs.'
              : ' and tell us what you were doing.'}
          </p>
          {requestId ? (
            <p className="results-state-detail">request id: {requestId}</p>
          ) : null}
          <p>
            <Link to="/" className="btn-primary">
              Return to the front page <span className="arrow">→</span>
            </Link>
          </p>
        </div>
      </div>
    </SheetLayout>
  );
}
