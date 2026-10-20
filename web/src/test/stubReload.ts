import { vi } from 'vitest';

/**
 * Replaces `location.reload` for the test; undo with `vi.unstubAllGlobals()`.
 * jsdom's reload can't be spied on, and a plain stand-in for `location`
 * would freeze the address the router reads, so everything else forwards
 * to the real one.
 */
export function stubReload(reload: () => void) {
  const real = window.location;
  vi.stubGlobal(
    'location',
    new Proxy({} as Location, {
      get: (_, prop) => {
        if (prop === 'reload') return reload;
        const value: unknown = Reflect.get(real, prop, real);
        return typeof value === 'function' ? value.bind(real) : value;
      },
      set: (_, prop, value) => Reflect.set(real, prop, value, real),
    }),
  );
}
