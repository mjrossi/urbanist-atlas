import type { UseQueryResult } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { ApiError } from '../lib/api.ts';
import { QueryState } from './QueryState.tsx';

function mkQuery<T>(
  partial: Partial<UseQueryResult<T, ApiError>>,
): UseQueryResult<T, ApiError> {
  return partial as UseQueryResult<T, ApiError>;
}

describe('QueryState', () => {
  it('renders the loading state with role=status when pending', () => {
    render(
      <QueryState query={mkQuery<string>({ isPending: true })} loading="Fetching">
        {(d) => <p>{d}</p>}
      </QueryState>,
    );
    const node = screen.getByRole('status');
    expect(node.textContent).toBe('Fetching');
  });

  it('renders the default error view with request id when isError', () => {
    const apiErr = new ApiError(500, 'Boom', undefined, 'rid-123');
    render(
      <QueryState query={mkQuery<string>({ isError: true, error: apiErr })}>
        {(d) => <p>{d}</p>}
      </QueryState>,
    );
    const node = screen.getByRole('alert');
    expect(node.textContent).toContain('Boom');
    expect(node.textContent).toContain('rid-123');
  });

  it('falls through to the default error view when the error callback returns undefined', () => {
    const apiErr = new ApiError(500, 'Boom', undefined, undefined);
    render(
      <QueryState
        query={mkQuery<string>({ isError: true, error: apiErr })}
        error={(e) => (e.status === 404 ? <div>Custom 404</div> : undefined)}
      >
        {(d) => <p>{d}</p>}
      </QueryState>,
    );
    expect(screen.getByRole('alert')).not.toBeNull();
  });

  it('uses the error callback when it returns JSX', () => {
    const apiErr = new ApiError(404, 'Not found', undefined, undefined);
    render(
      <QueryState
        query={mkQuery<string>({ isError: true, error: apiErr })}
        error={(e) => (e.status === 404 ? <div>Custom 404</div> : undefined)}
      >
        {(d) => <p>{d}</p>}
      </QueryState>,
    );
    expect(screen.getByText('Custom 404')).not.toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('renders the empty state when data matches `when`', () => {
    render(
      <QueryState
        query={mkQuery<string[]>({ data: [] })}
        empty={{ when: (xs) => xs.length === 0, render: <p>Nothing here</p> }}
      >
        {(xs) => (
          <ul>
            {xs.map((x) => (
              <li key={x}>{x}</li>
            ))}
          </ul>
        )}
      </QueryState>,
    );
    expect(screen.getByText('Nothing here')).not.toBeNull();
  });

  it('renders children when data is present and not empty', () => {
    render(
      <QueryState query={mkQuery<string>({ data: 'hello' })}>
        {(d) => <p>{d}</p>}
      </QueryState>,
    );
    expect(screen.getByText('hello')).not.toBeNull();
  });

  it('applies an extra className to the default loading container', () => {
    render(
      <QueryState query={mkQuery<string>({ isPending: true })} className="mt-48">
        {(d) => <p>{d}</p>}
      </QueryState>,
    );
    const node = screen.getByRole('status');
    expect(node.classList.contains('results-state')).toBe(true);
    expect(node.classList.contains('mt-48')).toBe(true);
  });

  it('applies an extra className to the default error container', () => {
    const apiErr = new ApiError(500, 'Boom', undefined, undefined);
    render(
      <QueryState
        query={mkQuery<string>({ isError: true, error: apiErr })}
        className="mt-48"
      >
        {(d) => <p>{d}</p>}
      </QueryState>,
    );
    const node = screen.getByRole('alert');
    expect(node.classList.contains('results-state')).toBe(true);
    expect(node.classList.contains('error')).toBe(true);
    expect(node.classList.contains('mt-48')).toBe(true);
  });
});
