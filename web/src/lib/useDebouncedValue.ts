import { useEffect, useState } from 'react';

/**
 * Returns `value` delayed by `delayMs` — it updates only after the
 * input has stayed unchanged for that long. Used to throttle the
 * region search query so a fast typist fires one request after they
 * pause, not one per keystroke.
 */
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => {
      setDebounced(value);
    }, delayMs);
    return () => {
      clearTimeout(id);
    };
  }, [value, delayMs]);
  return debounced;
}
