import type { ComponentType } from 'react';

/**
 * Recovery for a route chunk that fails to load, most often because a
 * deploy replaced the hashed chunk an open tab still points at. The
 * route's lazy loader tags the failure, and the root error boundary,
 * which renders at the URL the failed navigation committed, reloads that
 * URL once.
 */

const RELOAD_FLAG = 'atlas:chunk-reload';

/** A route module's dynamic import rejected. */
export class ChunkLoadError extends Error {
  constructor(cause: unknown) {
    super('A route chunk failed to load', { cause });
    this.name = 'ChunkLoadError';
  }
}

/**
 * A route `lazy` loader for one component export. It is the function
 * form, not `lazy: { Component }`: React Router drops a rejected lazy
 * `Component` property and renders the route empty, while a rejected
 * lazy function reaches the error boundary (staleChunk.test pins both).
 */
export function lazyComponent<M>(
  load: () => Promise<M>,
  pick: (mod: M) => ComponentType,
): () => Promise<{ Component: ComponentType }> {
  return async () => {
    let mod: M;
    try {
      mod = await load();
    } catch (err) {
      throw new ChunkLoadError(err);
    }
    try {
      sessionStorage.removeItem(RELOAD_FLAG);
    } catch {
      // Without storage there is no flag to clear.
    }
    return { Component: pick(mod) };
  };
}

/**
 * Whether a reload may be tried: storage works, so the flag can stop a
 * loop, and no reload has been tried since the last chunk loaded.
 */
export function canReloadForChunk(): boolean {
  try {
    return sessionStorage.getItem(RELOAD_FLAG) === null;
  } catch {
    return false;
  }
}

/** Reloads the page unless a reload was already tried or can't be guarded. */
export function reloadForChunk(): void {
  try {
    if (sessionStorage.getItem(RELOAD_FLAG) !== null) return;
    sessionStorage.setItem(RELOAD_FLAG, '1');
  } catch {
    return;
  }
  window.location.reload();
}
