import { useState, useEffect, useCallback, useRef } from 'react';
import { api } from '../api/client';

interface UseApiState<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
}

interface UseApiOptions {
  immediate?: boolean;
  refreshInterval?: number;
}

/** Generic hook for GET requests with auto-fetch and optional polling. */
export function useApi<T>(path: string, options: UseApiOptions = {}) {
  const { immediate = true, refreshInterval } = options;
  const [state, setState] = useState<UseApiState<T>>({
    data: null,
    error: null,
    loading: immediate,
  });
  const intervalRef = useRef<ReturnType<typeof setInterval>>(undefined);
  const abortRef = useRef<AbortController | null>(null);

  const fetch = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setState(prev => ({ ...prev, loading: true, error: null }));
    try {
      const response = await api.get<T>(path, { signal: controller.signal });
      if (controller.signal.aborted) return;
      setState({ data: response, error: null, loading: false });
    } catch (err) {
      if (controller.signal.aborted) return;
      const message = err instanceof Error ? err.message : 'Request failed';
      setState(prev => ({ ...prev, error: message, loading: false }));
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
    }
  }, [path]);

  useEffect(() => {
    if (!immediate) return;
    const id = setTimeout(fetch, 0);
    return () => clearTimeout(id);
  }, [fetch, immediate]);

  useEffect(() => {
    if (refreshInterval && refreshInterval > 0) {
      intervalRef.current = setInterval(fetch, refreshInterval);
      return () => clearInterval(intervalRef.current);
    }
  }, [fetch, refreshInterval]);

  useEffect(() => {
    return () => abortRef.current?.abort();
  }, []);

  return { ...state, refetch: fetch };
}

/** Hook for mutations (POST/PUT/PATCH/DELETE). */
export function useMutation<TInput = unknown, TOutput = unknown>(
  method: 'post' | 'put' | 'patch' | 'delete',
  path: string,
) {
  const [state, setState] = useState<UseApiState<TOutput>>({
    data: null,
    error: null,
    loading: false,
  });

  const mutate = useCallback(async (body?: TInput) => {
    setState({ data: null, error: null, loading: true });
    try {
      const response = method === 'delete'
        ? await api.delete<TOutput>(path)
        : await api[method]<TOutput>(path, body);
      setState({ data: response, error: null, loading: false });
      return response;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Request failed';
      setState(prev => ({ ...prev, error: message, loading: false }));
      throw err;
    }
  }, [method, path]);

  return { ...state, mutate };
}
