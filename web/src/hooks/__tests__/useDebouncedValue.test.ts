import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useDebouncedValue } from '../useDebouncedValue';

beforeEach(() => {
  vi.useFakeTimers();
});

describe('useDebouncedValue', () => {
  it('returns the initial value immediately', () => {
    const { result } = renderHook(() => useDebouncedValue('hello'));
    expect(result.current).toBe('hello');
  });

  it('does not update before the delay', () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 300),
      { initialProps: { value: 'a' } },
    );

    rerender({ value: 'ab' });
    act(() => { vi.advanceTimersByTime(200); });

    expect(result.current).toBe('a');
  });

  it('updates after the delay', () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 300),
      { initialProps: { value: 'a' } },
    );

    rerender({ value: 'ab' });
    act(() => { vi.advanceTimersByTime(300); });

    expect(result.current).toBe('ab');
  });

  it('resets the timer on rapid changes', () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 250),
      { initialProps: { value: '' } },
    );

    rerender({ value: 'a' });
    act(() => { vi.advanceTimersByTime(100); });

    rerender({ value: 'ab' });
    act(() => { vi.advanceTimersByTime(100); });

    rerender({ value: 'abc' });
    act(() => { vi.advanceTimersByTime(100); });

    // Only 100ms since last change — should still be ''
    expect(result.current).toBe('');

    act(() => { vi.advanceTimersByTime(150); });
    expect(result.current).toBe('abc');
  });

  it('uses default 250ms delay', () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value),
      { initialProps: { value: 'x' } },
    );

    rerender({ value: 'y' });
    act(() => { vi.advanceTimersByTime(249); });
    expect(result.current).toBe('x');

    act(() => { vi.advanceTimersByTime(1); });
    expect(result.current).toBe('y');
  });

  it('cleans up timeout on unmount', () => {
    const { result, rerender, unmount } = renderHook(
      ({ value }) => useDebouncedValue(value, 300),
      { initialProps: { value: 'init' } },
    );

    rerender({ value: 'changed' });
    unmount();
    act(() => { vi.advanceTimersByTime(300); });

    // After unmount the last rendered value should remain unchanged
    expect(result.current).toBe('init');
  });

  it('works with non-string types', () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebouncedValue(value, 100),
      { initialProps: { value: 42 } },
    );

    rerender({ value: 99 });
    act(() => { vi.advanceTimersByTime(100); });
    expect(result.current).toBe(99);
  });
});
