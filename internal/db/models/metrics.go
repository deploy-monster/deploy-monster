package models

import "time"

// MetricPoint represents a single metric data point.
type MetricPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// ServerMetrics stores server-level metrics.
type ServerMetrics struct {
	ID        string    `json:"id"`
	ServerID  string    `json:"server_id"`
	CPUUsage  float64   `json:"cpu_usage"`   // percentage
	MemUsage  float64   `json:"mem_usage"`   // percentage
	DiskUsage float64   `json:"disk_usage"`  // percentage
	NetRx     int64     `json:"net_rx"`      // bytes
	NetTx     int64     `json:"net_tx"`      // bytes
	LoadAvg   []float64 `json:"load_avg"`    // 1, 5, 15 min
	RecordedAt time.Time `json:"recorded_at"`
}

// ContainerMetrics stores container-level metrics.
type ContainerMetrics struct {
	ID          string    `json:"id"`
	ContainerID string    `json:"container_id"`
	AppID       string    `json:"app_id,omitempty"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryMB    float64   `json:"memory_mb"`
	MemoryLimit float64   `json:"memory_limit_mb"`
	NetworkRx   int64     `json:"network_rx"`
	NetworkTx   int64     `json:"network_tx"`
	BlockRead   int64     `json:"block_read"`
	BlockWrite  int64     `json:"block_write"`
	PIDs        int       `json:"pids"`
	Status      string    `json:"status"` // running, stopped, paused
	RecordedAt  time.Time `json:"recorded_at"`
}