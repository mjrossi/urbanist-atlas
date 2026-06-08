import { afterEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from './api.ts';
import { reportClientError, requestIdOf } from './clientErrors.ts';

describe('requestIdOf', () => {
  it('returns the request id for an ApiError that has one', () => {
    const err = new ApiError(500, 'boom', undefined, 'rid-123');
    expect(requestIdOf(err)).toBe('rid-123');
  });

  it('returns undefined for an ApiError without a request id', () => {
    const err = new ApiError(500, 'boom', undefined, undefined);
    expect(requestIdOf(err)).toBeUndefined();
  });

  it('returns undefined for non-ApiError values', () => {
    expect(requestIdOf(new Error('plain'))).toBeUndefined();
    expect(requestIdOf('a string')).toBeUndefined();
    expect(requestIdOf(undefined)).toBeUndefined();
  });
});

describe('reportClientError', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('logs the context and request id for an ApiError (dev mode)', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const err = new ApiError(503, 'down', undefined, 'rid-xyz');

    reportClientError('query', err);

    expect(spy).toHaveBeenCalledTimes(1);
    expect(spy).toHaveBeenCalledWith(
      expect.stringMatching(/query.*request id: rid-xyz/),
      err,
    );
  });

  it('logs without a request id for a plain error', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const err = new Error('kaboom');

    reportClientError('mutation', err);

    expect(spy).toHaveBeenCalledTimes(1);
    expect(spy).toHaveBeenCalledWith(expect.not.stringContaining('request id'), err);
  });
});
