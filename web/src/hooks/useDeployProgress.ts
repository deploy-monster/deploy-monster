import { useCallback, useEffect, useRef, useState } from 'react';
import type { DeployStatus } from '@/components/topology/DeployModal';
import type { TopologyDeployResponse } from '@/types/topology';

interface DeployProgressMessage {
  type: 'deploy_progress';
  projectId: string;
  stage: DeployStatus;
  message: string;
  progress: number;
  timestamp: number;
}

interface DeployCompleteMessage {
  type: 'deploy_complete';
  projectId: string;
  success: boolean;
  message: string;
  duration: string;
  containers?: string[];
  networks?: string[];
  volumes?: string[];
  errors?: string[];
  timestamp: number;
}

type DeployWSMessage = DeployProgressMessage | DeployCompleteMessage;

interface UseDeployProgressOptions {
  projectId: string;
  enabled?: boolean;
  onComplete?: (result: TopologyDeployResponse) => void;
  onProgress?: (progress: DeployProgressMessage) => void;
}

interface UseDeployProgressReturn {
  status: DeployStatus;
  progress: number;
  message: string;
  result: TopologyDeployResponse | null;
  isConnected: boolean;
  reset: () => void;
}

/** Hook for real-time deployment progress via WebSocket */
export function useDeployProgress({
  projectId,
  enabled = true,
  onComplete,
  onProgress,
}: UseDeployProgressOptions): UseDeployProgressReturn {
  const [status, setStatus] = useState<DeployStatus>('idle');
  const [progress, setProgress] = useState(0);
  const [message, setMessage] = useState('');
  const [result, setResult] = useState<TopologyDeployResponse | null>(null);
  const [isConnected, setIsConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  const reset = useCallback(() => {
    setStatus('idle');
    setProgress(0);
    setMessage('');
    setResult(null);
  }, []);

  useEffect(() => {
    const connect = () => {
      if (!enabled || !projectId) return;

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/api/v1/topology/deploy/${projectId}/progress`;

      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        setIsConnected(true);
      };

      ws.onmessage = (event) => {
        try {
          const data: DeployWSMessage = JSON.parse(event.data);

          if (data.type === 'deploy_progress') {
            setStatus(data.stage);
            setProgress(data.progress);
            setMessage(data.message);
            onProgress?.(data);
          } else if (data.type === 'deploy_complete') {
            setStatus(data.success ? 'success' : 'error');
            setProgress(100);
            setMessage(data.message);
            const response: TopologyDeployResponse = {
              success: data.success,
              message: data.message,
              duration: data.duration,
              containers: data.containers,
              networks: data.networks,
              volumes: data.volumes,
              errors: data.errors,
            };
            setResult(response);
            onComplete?.(response);
          }
        } catch (err) {
          console.error('Failed to parse deploy progress message:', err);
        }
      };

      ws.onclose = () => {
        setIsConnected(false);
        // Auto-reconnect after 3 seconds if still enabled
        if (enabled) {
          reconnectTimeoutRef.current = setTimeout(connect, 3000);
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
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
    };
  }, [enabled, projectId, onComplete, onProgress]);

  return {
    status,
    progress,
    message,
    result,
    isConnected,
    reset,
  };
}
