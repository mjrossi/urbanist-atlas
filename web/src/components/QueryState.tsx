import type { UseQueryResult } from '@tanstack/react-query';
import type { ReactNode } from 'react';

import { ApiError } from '../lib/api.ts';

interface Empty<T> {
  when: (data: T) => boolean;
  render: ReactNode;
}

interface Props<T> {
  query: UseQueryResult<T, ApiError>;
  /** Pending-state content. Defaults to "Loading…". */
  loading?: ReactNode;
  /** Custom error view; return `undefined` for the default one. */
  error?: (e: ApiError) => ReactNode | undefined;
  empty?: Empty<T>;
  /** Appended to the default loading and error containers. */
  className?: string;
  children: (data: T) => ReactNode;
}

/**
 * Renders a `useQuery` result's loading, error, empty or success state,
 * with one shared look for loading and errors.
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
