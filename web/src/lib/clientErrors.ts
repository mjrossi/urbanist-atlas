import { ApiError } from './api.ts';

/**
 * The server's `X-Request-ID` for an {@link ApiError}, else undefined.
 * See `docs/superpowers/specs/2026-06-08-observability-design.md`.
 */
export function requestIdOf(error: unknown): string | undefined {
  return error instanceof ApiError ? error.requestId : undefined;
}

/**
 * Logs an unexpected client error, with its request id, in dev only:
 * the app ships no client error tracking.
 */
export function reportClientError(context: string, error: unknown): void {
  if (!import.meta.env.DEV) return;
  const rid = requestIdOf(error);
  console.error(`[atlas] ${context}${rid ? ` (request id: ${rid})` : ''}`, error);
}

/**
 * Dev only: logs window `error` and `unhandledrejection` events, which
 * escape React's error boundaries.
 */
export function installGlobalErrorLogging(): void {
  if (!import.meta.env.DEV) return;
  window.addEventListener('error', (event) => {
    reportClientError('window.onerror', event.error ?? event.message);
  });
  window.addEventListener('unhandledrejection', (event) => {
    reportClientError('unhandledrejection', event.reason);
  });
}
