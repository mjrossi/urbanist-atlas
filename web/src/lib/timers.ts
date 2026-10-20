import { useEffect, useState } from 'react';

// Lives here, not in Submit.tsx, so it can be unit-tested: a component
// module can't export non-component helpers without tripping
// react-refresh/only-export-components.

/**
 * A per-second countdown. `seconds` ticks down to zero; the returned
 * setter (re)arms it from a server-provided Retry-After. Backs the 429
 * lockout — the submit button stays disabled until it reaches zero.
 */
export function useCountdown() {
  const [seconds, setSeconds] = useState(0);
  useEffect(() => {
    if (seconds <= 0) return;
    const id = setInterval(() => {
      setSeconds((s) => (s <= 1 ? 0 : s - 1));
    }, 1000);
    return () => {
      clearInterval(id);
    };
  }, [seconds]);
  return [seconds, setSeconds] as const;
}
