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

  const fetch = useCallback(async () => {
    setState(prev => ({ ...prev, loading: true, error: null }));
    try {
      const response = await api.get<T>(path);
      setState({ data: response, error: null, loading: false });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Request failed';
      setState(prev => ({ ...prev, error: message, loading: false }));
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

/** Paginated list hook with page controls. */
export function usePaginatedApi<T>(basePath: string, perPage = 20) {
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);

  const path = `${basePath}?page=${page}&per_page=${perPage}`;

  const { data, error, loading, refetch } = useApi<T>(path);

  const nextPage = useCallback(() => {
    if (page < totalPages) setPage(p => p + 1);
  }, [page, totalPages]);

  const prevPage = useCallback(() => {
    if (page > 1) setPage(p => p - 1);
  }, [page]);

  const goToPage = useCallback((p: number) => {
    if (p >= 1 && p <= totalPages) setPage(p);
  }, [totalPages]);

  return {
    data, error, loading, refetch,
    page, totalPages, nextPage, prevPage, goToPage,
    setTotalPages,
  };
}
