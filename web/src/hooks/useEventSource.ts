import { useState, useEffect, useRef, useCallback } from 'react';

interface UseEventSourceOptions {
  /** Auto-reconnect on error (default: true) */
  reconnect?: boolean;
  /** Reconnect delay in ms (default: 5000) */
  reconnectDelay?: number;
}

/** Hook for Server-Sent Events (SSE) — used for log streaming and real-time updates. */
export function useEventSource<T = string>(
  url: string | null,
  options: UseEventSourceOptions = {},
) {
  const { reconnect = true, reconnectDelay = 5000 } = options;
  const [data, setData] = useState<T[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const esRef = useRef<EventSource | null>(null);

  const connect = useCallback(() => {
    if (!url) return;

    const token = localStorage.getItem('access_token');
    const separator = url.includes('?') ? '&' : '?';
    const fullUrl = token ? `${url}${separator}token=${token}` : url;

    const es = new EventSource(fullUrl);
    esRef.current = es;

    es.onopen = () => {
      setIsConnected(true);
      setError(null);
    };

    es.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data) as T;
        setData(prev => [...prev, parsed]);
      } catch {
        setData(prev => [...prev, event.data as T]);
      }
    };

    es.onerror = () => {
      setIsConnected(false);
      es.close();
      if (reconnect) {
        setError('Connection lost, reconnecting...');
        setTimeout(connect, reconnectDelay);
      } else {
        setError('Connection lost');
      }
    };
  }, [url, reconnect, reconnectDelay]);

  useEffect(() => {
    connect();
    return () => {
      esRef.current?.close();
    };
  }, [connect]);

  const clear = useCallback(() => setData([]), []);

  const close = useCallback(() => {
    esRef.current?.close();
    setIsConnected(false);
  }, []);

  return { data, isConnected, error, clear, close };
}
