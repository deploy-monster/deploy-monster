package models

import "time"

// AlertRule defines conditions for triggering alerts.
type AlertRule struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	MetricType   string    `json:"metric_type"`   // cpu, memory, disk, container_status
	TargetType   string    `json:"target_type"`   // app, server, container
	TargetID     string    `json:"target_id,omitempty"`
	Condition    string    `json:"condition"`    // gt, lt, eq, gte, lte
	Threshold    float64   `json:"threshold"`
	WindowSeconds int      `json:"window_seconds"` // time window for evaluation
	CooldownSeconds int    `json:"cooldown_seconds"` // minimum time between alerts
	Severity     string    `json:"severity"`    // info, warning, critical
	Enabled      bool      `json:"enabled"`
	NotifyChannels []string `json:"notify_channels"` // email, slack, telegram, discord
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Alert tracks triggered alerts.
type Alert struct {
	ID         string    `json:"id"`
	RuleID     string    `json:"rule_id"`
	TenantID   string    `json:"tenant_id"`
	TargetID   string    `json:"target_id"`
	TargetName string    `json:"target_name"`
	Message    string    `json:"message"`
	Severity   string    `json:"severity"` // info, warning, critical
	Status     string    `json:"status"`   // firing, resolved
	Value      float64   `json:"value"`    // metric value that triggered
	Threshold  float64   `json:"threshold"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	FiredAt    time.Time `json:"fired_at"`
}

// NotificationChannel stores notification configuration.
type NotificationChannel struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"` // email, slack, telegram, discord, webhook
	Config    string    `json:"config"` // JSON config (webhook URL, email addresses, etc.)
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}