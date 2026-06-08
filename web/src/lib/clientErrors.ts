import { ApiError } from './api.ts';

/**
 * The server's `X-Request-ID` for an {@link ApiError}, else undefined.
 * The API threads the same id through every log line, so it is the
 * handle for correlating a client-side failure to the server logs — the
 * project's deliberate, SaaS-free debugging path (see
 * `docs/superpowers/specs/2026-06-08-observability-design.md`).
 */
export function requestIdOf(error: unknown): string | undefined {
  return error instanceof ApiError ? error.requestId : undefined;
}

/**
 * Dev-only structured console line for an unexpected client error,
 * including the request id when present so it can be grepped in the API
 * logs. No-ops in production by design: the app ships no client
 * error-tracking SaaS. Production error visibility is the graceful error
 * boundary (which surfaces the request id to the user) plus the server
 * logs the user's reported id points at.
 */
export function reportClientError(context: string, error: unknown): void {
  if (!import.meta.env.DEV) return;
  const rid = requestIdOf(error);
  console.error(`[atlas] ${context}${rid ? ` (request id: ${rid})` : ''}`, error);
}

/**
 * Registers window-level `error` + `unhandledrejection` logging (dev
 * only) so failures that escape React's render tree — async callbacks,
 * event handlers, rejected promises — still surface in the console with
 * any request id attached. No-ops in production.
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
