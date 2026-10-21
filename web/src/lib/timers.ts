import { useEffect, useState } from 'react';

/**
 * A per-second countdown to zero; the returned setter (re)arms it.
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
