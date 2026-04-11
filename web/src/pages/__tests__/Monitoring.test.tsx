import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

// -- Mocks -------------------------------------------------------------------
//
// The Monitoring page fires two parallel useApi() calls: /metrics/server
// (with refreshInterval) and /alerts. We route by path.

type ApiResponse = { data: unknown; loading: boolean };
const apiResponses: Record<string, ApiResponse> = {};
const refetchMetricsMock = vi.fn();

function setApi(path: string, data: unknown, loading = false) {
  apiResponses[path] = { data, loading };
}
function clearApi() {
  for (const k of Object.keys(apiResponses)) delete apiResponses[k];
}

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks');
  return {
    ...actual,
    useApi: (path: string) => {
      const res = apiResponses[path] ?? { data: null, loading: true };
      const refetch = path === '/metrics/server' ? refetchMetricsMock : vi.fn();
      return { data: res.data, loading: res.loading, error: null, refetch };
    },
  };
});

import { Monitoring } from '../Monitoring';

const metricsFixture = {
  cpu_percent: 42.5,
  memory_used: 4 * 1024 * 1024 * 1024, // 4 GB
  memory_total: 16 * 1024 * 1024 * 1024, // 16 GB
  disk_used: 120 * 1024 * 1024 * 1024, // 120 GB
  disk_total: 500 * 1024 * 1024 * 1024, // 500 GB
  network_rx: 1024 * 512, // 512 KB/s
  network_tx: 1024 * 256, // 256 KB/s
  load_avg: [0.8, 0.9, 1.0],
  containers_running: 5,
  containers_total: 7,
  uptime: 86400 * 3 + 3600 * 2, // 3d 2h
};

function renderMonitoring() {
  return render(
    <MemoryRouter>
      <Monitoring />
    </MemoryRouter>
  );
}

describe('Monitoring page', () => {
  beforeEach(() => {
    clearApi();
    refetchMetricsMock.mockReset();
  });

  it('renders the hero header with the Monitoring title and healthy badge', () => {
    setApi('/metrics/server', metricsFixture);
    setApi('/alerts', []);
    renderMonitoring();

    expect(screen.getByRole('heading', { name: /^monitoring$/i })).toBeInTheDocument();
    expect(screen.getByText(/all systems healthy/i)).toBeInTheDocument();
  });

  it('shows the destructive alert badge when there are firing alerts', () => {
    setApi('/metrics/server', metricsFixture);
    setApi('/alerts', [
      { id: 'a1', name: 'CPU High', metric: 'cpu', threshold: 90, status: 'firing' },
      { id: 'a2', name: 'Mem High', metric: 'memory', threshold: 80, status: 'firing' },
    ]);
    renderMonitoring();

    // Hero header badge shows "2 alerts firing"
    expect(screen.getByText(/2 alerts firing/i)).toBeInTheDocument();
  });

  it('renders the four MetricCard tiles with CPU, Memory, Disk, Network labels', () => {
    setApi('/metrics/server', metricsFixture);
    setApi('/alerts', []);
    renderMonitoring();

    expect(screen.getByText('CPU Usage')).toBeInTheDocument();
    expect(screen.getByText('Memory')).toBeInTheDocument();
    expect(screen.getByText('Disk')).toBeInTheDocument();
    expect(screen.getByText('Network')).toBeInTheDocument();
    // CPU percent value — shows in both the big value and the usage bar.
    expect(screen.getAllByText('42.5%').length).toBeGreaterThan(0);
    // Load average subtext
    expect(screen.getByText(/load: 0\.80/i)).toBeInTheDocument();
  });

  it('renders the secondary stat cards with containers ratio and uptime', () => {
    setApi('/metrics/server', metricsFixture);
    setApi('/alerts', []);
    renderMonitoring();

    // Containers card shows "5" as the running count
    expect(screen.getByText('Containers')).toBeInTheDocument();
    expect(screen.getByText('5')).toBeInTheDocument();
    // Uptime formatted as "3d 2h"
    expect(screen.getByText(/3d 2h/i)).toBeInTheDocument();
  });

  it('calls refetch on the metrics feed when Refresh is clicked', () => {
    setApi('/metrics/server', metricsFixture);
    setApi('/alerts', []);
    renderMonitoring();

    fireEvent.click(screen.getByRole('button', { name: /refresh/i }));
    expect(refetchMetricsMock).toHaveBeenCalled();
  });

  it('shows the Alerts empty state when there are no alert rules', () => {
    setApi('/metrics/server', metricsFixture);
    setApi('/alerts', []);
    renderMonitoring();

    // Alerts tab is the default; the empty state heading should be rendered.
    expect(
      screen.getByRole('heading', { name: /no alert rules configured/i })
    ).toBeInTheDocument();
  });

  it('renders each alert rule in a row when alerts are populated', () => {
    setApi('/metrics/server', metricsFixture);
    setApi('/alerts', [
      { id: 'a1', name: 'CPU High', metric: 'cpu', threshold: 90, status: 'ok' },
      { id: 'a2', name: 'Disk Low', metric: 'disk', threshold: 95, status: 'firing' },
    ]);
    renderMonitoring();

    expect(screen.getByText('CPU High')).toBeInTheDocument();
    expect(screen.getByText('Disk Low')).toBeInTheDocument();
  });

  it('renders the Prometheus endpoint hint when switching to the Prometheus tab', () => {
    setApi('/metrics/server', metricsFixture);
    setApi('/alerts', []);
    renderMonitoring();

    fireEvent.click(screen.getByRole('tab', { name: /prometheus/i }));

    // CardTitle is a <div>, not a heading — query by text instead.
    expect(screen.getByText(/prometheus endpoint/i)).toBeInTheDocument();
    expect(screen.getByText(/curl http:\/\/localhost:8443\/metrics/i)).toBeInTheDocument();
  });

  it('renders metric-card skeletons while the metrics feed is loading', () => {
    setApi('/metrics/server', null, true);
    setApi('/alerts', []);
    renderMonitoring();

    // CPU Usage label should not be rendered while loading.
    expect(screen.queryByText('CPU Usage')).not.toBeInTheDocument();
    // Header remains.
    expect(screen.getByRole('heading', { name: /^monitoring$/i })).toBeInTheDocument();
  });
});
