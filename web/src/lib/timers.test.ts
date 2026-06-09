import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useCooldown, useCountdown } from './timers.ts';

describe('useCountdown', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('starts at zero before it is armed', () => {
    const { result } = renderHook(() => useCountdown());
    expect(result.current[0]).toBe(0);
  });

  it('ticks down one second at a time once armed, then stops at zero', () => {
    const { result } = renderHook(() => useCountdown());

    // Arm it the way the 429 handler does — from a server Retry-After.
    act(() => {
      result.current[1](3);
    });
    expect(result.current[0]).toBe(3);

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current[0]).toBe(2);

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current[0]).toBe(1);

    // Final tick exercises the `s <= 1 ? 0` floor rather than `s - 1`.
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current[0]).toBe(0);

    // At zero the interval clears itself, so more time can't underflow it.
    act(() => {
      vi.advanceTimersByTime(5000);
    });
    expect(result.current[0]).toBe(0);
    expect(vi.getTimerCount()).toBe(0);
  });

  it('re-arms from a fresh value when a second 429 arrives mid-countdown', () => {
    const { result } = renderHook(() => useCountdown());

    act(() => {
      result.current[1](2);
    });
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current[0]).toBe(1);

    // A longer Retry-After lands before the first elapsed: restart from it.
    act(() => {
      result.current[1](9);
    });
    expect(result.current[0]).toBe(9);
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current[0]).toBe(8);
  });

  it('clears its interval on unmount', () => {
    const { result, unmount } = renderHook(() => useCountdown());
    act(() => {
      result.current[1](5);
    });
    expect(vi.getTimerCount()).toBeGreaterThan(0);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe('useCooldown', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('is inactive until triggered', () => {
    const { result } = renderHook(() => useCooldown(1500));
    expect(result.current[0]).toBe(false);
  });

  it('latches active on trigger, then clears itself after the window', () => {
    const { result } = renderHook(() => useCooldown(1500));

    act(() => {
      result.current[1]();
    });
    expect(result.current[0]).toBe(true);

    // Still locked one tick short of the window…
    act(() => {
      vi.advanceTimersByTime(1499);
    });
    expect(result.current[0]).toBe(true);

    // …and released exactly when it elapses.
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(result.current[0]).toBe(false);
  });

  it('clears its timeout on unmount', () => {
    const { result, unmount } = renderHook(() => useCooldown(1500));
    act(() => {
      result.current[1]();
    });
    expect(vi.getTimerCount()).toBeGreaterThan(0);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
