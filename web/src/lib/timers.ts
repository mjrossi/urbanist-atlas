import { useEffect, useState } from 'react';

// Two small timer hooks backing the /submit button's disabled state: a
// post-submit cooldown latch and a 429 Retry-After countdown. They live
// here, not in Submit.tsx, so they can be unit-tested in isolation — a
// component module can't export non-component helpers without tripping
// react-refresh/only-export-components (and `--max-warnings 0` fails on it).

/**
 * A latch that goes true when `trigger()` is called, then clears itself
 * after `ms`. Backs the brief post-submit lockout that stops a
 * triple-click from firing duplicate POSTs.
 */
export function useCooldown(ms: number) {
  const [active, setActive] = useState(false);
  useEffect(() => {
    if (!active) return;
    const id = setTimeout(() => {
      setActive(false);
    }, ms);
    return () => {
      clearTimeout(id);
    };
  }, [active, ms]);
  const trigger = () => {
    setActive(true);
  };
  return [active, trigger] as const;
}

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
