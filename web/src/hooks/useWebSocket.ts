import { useState, useEffect, useRef, useCallback } from 'react';

interface UseWebSocketOptions {
  /** Auto-reconnect on close (default: true) */
  reconnect?: boolean;
  /** Reconnect delay in ms (default: 3000) */
  reconnectDelay?: number;
  /** Max reconnect attempts (default: 10) */
  maxRetries?: number;
  /** Callback on message received */
  onMessage?: (data: unknown) => void;
}

interface UseWebSocketReturn {
  /** Send a message through the WebSocket */
  send: (data: unknown) => void;
  /** Last received message */
  lastMessage: unknown | null;
  /** Connection state */
  readyState: number;
  /** Whether the connection is open */
  isConnected: boolean;
  /** Close the connection */
  close: () => void;
}

/** Hook for WebSocket connections with auto-reconnect. */
export function useWebSocket(
  url: string,
  options: UseWebSocketOptions = {},
): UseWebSocketReturn {
  const {
    reconnect = true,
    reconnectDelay = 3000,
    maxRetries = 10,
    onMessage,
  } = options;

  const [lastMessage, setLastMessage] = useState<unknown | null>(null);
  const [readyState, setReadyState] = useState<number>(WebSocket.CONNECTING);
  const wsRef = useRef<WebSocket | null>(null);
  const retriesRef = useRef(0);
  const onMessageRef = useRef(onMessage);

  useEffect(() => {
    onMessageRef.current = onMessage;
  }, [onMessage]);

  useEffect(() => {
    const connect = () => {
      // Cookies are auto-sent on same-origin WebSocket handshake
      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = () => {
        setReadyState(WebSocket.OPEN);
        retriesRef.current = 0;
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          setLastMessage(data);
          onMessageRef.current?.(data);
        } catch {
          setLastMessage(event.data);
          onMessageRef.current?.(event.data);
        }
      };

      ws.onclose = () => {
        setReadyState(WebSocket.CLOSED);
        if (reconnect && retriesRef.current < maxRetries) {
          retriesRef.current++;
          setTimeout(connect, reconnectDelay);
        }
      };

      ws.onerror = () => {
        ws.close();
      };
    };

    connect();
    return () => {
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [url, reconnect, reconnectDelay, maxRetries, onMessage]);

  const send = useCallback((data: unknown) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(typeof data === 'string' ? data : JSON.stringify(data));
    }
  }, []);

  const close = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close();
    }
  }, []);

  return {
    send,
    lastMessage,
    readyState,
    isConnected: readyState === WebSocket.OPEN,
    close,
  };
}
