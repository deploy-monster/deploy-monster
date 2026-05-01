export interface ServerMetrics {
  cpu_percent: number;
  memory_used: number;
  memory_total: number;
  disk_used: number;
  disk_total: number;
  network_rx: number;
  network_tx: number;
  uptime: number;
  containers_running: number;
  containers_total: number;
  load_avg: number[];
}

export interface AlertRule {
  id: string;
  name: string;
  metric: string;
  threshold: number;
  status: 'ok' | 'firing' | 'resolved';
  last_checked: string;
}
