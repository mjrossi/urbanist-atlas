import { useEffect, useState } from 'react';
import { isRouteErrorResponse, Link, useLocation, useRouteError } from 'react-router';

import { PageBreadcrumb } from '../components/PageBreadcrumb.tsx';
import { SheetLayout } from '../components/SheetLayout.tsx';
import { reportClientError, requestIdOf } from '../lib/clientErrors.ts';
import { canReloadForChunk, ChunkLoadError, reloadForChunk } from '../lib/staleChunk.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import { NotFoundWithLayout } from './NotFound.tsx';

/**
 * The root `errorElement`. React Router routes BOTH unmatched URLs (a
 * 404 ErrorResponse) and any unhandled error thrown by a descendant
 * component or loader here. We branch on which it is: a 404 keeps the
 * newspaper "page not in this edition" page; a route chunk that failed
 * to load gets one reload; anything else is an unexpected failure and
 * gets its own "stop press" page rather than masquerading as a 404.
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
  if (error instanceof ChunkLoadError) {
    return <StaleChunk error={error} />;
  }
  return <InternalErrorWithLayout error={error} />;
}

/**
 * A route chunk failed to load, usually because a deploy replaced it.
 * The failed navigation's URL is already the address, so one reload
 * fetches the new build at the page the reader asked for. If that was
 * already tried, or can't be guarded against looping, the reader gets a
 * page with a link that does a full load.
 */
function StaleChunk({ error }: { error: ChunkLoadError }) {
  // Decided once, so the flag that reloadForChunk sets can't flip this
  // render to the error page while the reload is under way.
  const [reloading] = useState(canReloadForChunk);
  useEffect(() => {
    if (reloading) reloadForChunk();
  }, [reloading]);
  if (reloading) return <SheetLayout>{null}</SheetLayout>;
  return <StaleChunkWithLayout error={error} />;
}

function StaleChunkWithLayout({ error }: { error: ChunkLoadError }) {
  useDocumentTitle('A new edition is out — Urbanist Atlas');
  useEffect(() => {
    reportClientError('route chunk failed to load', error);
  }, [error]);
  const { pathname, search } = useLocation();
  return (
    <SheetLayout>
      <PageBreadcrumb
        prefix={[{ label: 'Atlas', to: '/' }]}
        current="Error"
        meta="Stop press"
      />
      <div className="lede mt-56">
        <div className="eyebrow">
          § Stop press
          <span className="eyebrow-rule" />
        </div>
        <h1>
          A new edition <span className="accent">is out.</span>
        </h1>
        <p className="deck">
          The atlas was updated while this page was open, and part of it couldn&rsquo;t
          load. Reloading picks up the new edition.
        </p>
      </div>
      <div className="spread mt-24">
        <div className="prose">
          <p>
            {/* A plain link, not a router Link, so the browser fetches the
                new build instead of running the stale one. */}
            <a href={pathname + search} className="btn-primary">
              Reload the page <span className="arrow">→</span>
            </a>
          </p>
        </div>
      </div>
    </SheetLayout>
  );
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
