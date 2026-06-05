import type { UseQueryResult } from '@tanstack/react-query';
import type { ReactNode } from 'react';

import { ApiError } from '../lib/api.ts';

interface Empty<T> {
  when: (data: T) => boolean;
  render: ReactNode;
}

interface Props<T> {
  query: UseQueryResult<T, ApiError>;
  /** Text or JSX rendered inside `<p className="results-state" role="status">` while pending. Defaults to "Loading…". */
  loading?: ReactNode;
  /**
   * Optional error renderer. Return JSX for a custom message (e.g.
   * a 404 lede) — or return `undefined` to fall through to the
   * default `<p className="results-state error" role="alert">` view.
   */
  error?: (e: ApiError) => ReactNode | undefined;
  /** Optional empty-data state — rendered when `when(data)` is true. */
  empty?: Empty<T>;
  /**
   * Optional extra className appended to the default loading/error
   * containers. Use a spacing utility (e.g. `mt-48`) when a route
   * needs vertical breathing room above the state row.
   */
  className?: string;
  /** Renderer for the success state. */
  children: (data: T) => ReactNode;
}

/**
 * QueryState collapses the loading / error / empty / success triad
 * that every TanStack `useQuery` consumer ends up writing by hand.
 * One definition of "what each state looks like" — the same
 * `<p className="results-state">` wrapper, the same role attributes,
 * the same `request_id` detail line on errors.
 *
 * For routes that need a custom error shape (Region.tsx and Org.tsx
 * render a full-JSX "not in the atlas yet" lede on 404), pass an
 * `error` callback that returns JSX for the special cases and
 * `undefined` for everything else — undefined falls through to the
 * default presentation.
 */
export function QueryState<T>({
  query,
  loading,
  error,
  empty,
  className,
  children,
}: Props<T>): ReactNode {
  const loadingClass = className ? `results-state ${className}` : 'results-state';
  const errorClass = className
    ? `results-state error ${className}`
    : 'results-state error';

  if (query.isPending) {
    return (
      <p className={loadingClass} role="status">
        {loading ?? 'Loading…'}
      </p>
    );
  }
  if (query.isError) {
    const custom = error?.(query.error);
    if (custom !== undefined && custom !== null) return custom;
    return (
      <p className={errorClass} role="alert">
        {query.error.message}
        {query.error.requestId ? (
          <span className="results-state-detail">
            request id: {query.error.requestId}
          </span>
        ) : null}
      </p>
    );
  }
  if (empty?.when(query.data)) {
    return empty.render;
  }
  return children(query.data);
}
