import { useEffect, useState, useRef } from 'react';
import { cn } from '@/lib/utils';

interface AppLogsProps {
  appId: string;
}

const SAFE_ID_PATTERN = /^[a-zA-Z0-9_-]+$/;

export function AppLogs({ appId }: AppLogsProps) {
  const [logs, setLogs] = useState<string[]>([]);
  const [connected, setConnected] = useState(false);
  const [streamError, setStreamError] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const hasInvalidAppId = appId !== '' && !SAFE_ID_PATTERN.test(appId);
  const visibleStreamError = hasInvalidAppId ? 'Invalid application ID' : streamError;
  const visibleConnected = hasInvalidAppId ? false : connected;
  const visibleLogs = hasInvalidAppId ? [] : logs;

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs]);

  useEffect(() => {
    if (!appId || hasInvalidAppId) return;

    // Validate appId to prevent URL injection attacks
    if (!SAFE_ID_PATTERN.test(appId)) return;

    const eventSource = new EventSource(`/api/v1/apps/${appId}/logs/stream`);

    eventSource.onopen = () => {
      setConnected(true);
      setStreamError(null);
    };

    eventSource.onmessage = (e) => {
      setLogs((prev) => [...prev.slice(-500), e.data]);
    };

    eventSource.addEventListener('error', (ev) => {
      const data = (ev as MessageEvent).data;
      if (typeof data === 'string' && data.length > 0) {
        setStreamError(data);
      } else if (eventSource.readyState === EventSource.CLOSED) {
        setStreamError('Stream closed');
      }
      setConnected(false);
    });

    return () => {
      eventSource.close();
      setConnected(false);
    };
  }, [appId, hasInvalidAppId]);

  return (
    <div className="rounded-lg border bg-card overflow-hidden">
      <div className="rounded-lg bg-[#0d1117]">
        {/* Terminal header */}
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/5 bg-[#161b22]">
          <div className="flex items-center gap-3">
            <div className="flex gap-1.5">
              <div className="size-3 rounded-full bg-[#ff5f57]" />
              <div className="size-3 rounded-full bg-[#febc2e]" />
              <div className="size-3 rounded-full bg-[#28c840]" />
            </div>
            <span className="text-xs text-[#8b949e] font-mono">Container Logs</span>
          </div>
          <div className="flex items-center gap-2">
            <span
              className={cn(
                'size-2 rounded-full transition-colors',
                visibleConnected ? 'bg-[#28c840] shadow-sm shadow-[#28c840]/50' : 'bg-[#8b949e]'
              )}
            />
            <span className="text-xs text-[#8b949e] font-mono">
              {visibleConnected ? 'Live' : 'Disconnected'}
            </span>
          </div>
        </div>

        {/* Log content */}
        <div
          ref={scrollRef}
          className="h-[28rem] overflow-auto p-4 font-mono text-sm leading-relaxed scroll-smooth"
        >
          {visibleLogs.length === 0 ? (
            <div className="flex items-center gap-2 text-[#8b949e]">
              {visibleStreamError ? (
                <>
                  <div className="size-1.5 rounded-full bg-[#ff5f57]" />
                  <span>{visibleStreamError}</span>
                </>
              ) : (
                <>
                  <div className="size-1.5 rounded-full bg-[#8b949e] animate-pulse" />
                  <span>Waiting for logs...</span>
                </>
              )}
            </div>
          ) : (
            visibleLogs.map((line, i) => (
              <div
                key={i}
                className="text-[#c9d1d9] hover:bg-[#161b22] px-2 -mx-2 py-px rounded group"
              >
                <span className="text-[#484f58] select-none mr-4 inline-block w-10 text-right group-hover:text-[#6e7681]">
                  {String(i + 1).padStart(4, ' ')}
                </span>
                {line}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
