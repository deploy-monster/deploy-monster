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

  useEffect(() => {
    const connect = () => {
      if (!url) return;

      // withCredentials sends cookies for same-origin SSE connections
      const es = new EventSource(url, { withCredentials: true });
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
    };

    connect();
    return () => {
      esRef.current?.close();
    };
  }, [url, reconnect, reconnectDelay]);

  const clear = useCallback(() => setData([]), []);

  const close = useCallback(() => {
    esRef.current?.close();
    setIsConnected(false);
  }, []);

  return { data, isConnected, error, clear, close };
}
