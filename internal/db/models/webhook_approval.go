package models

import "time"

// WebhookApproval represents a pending approval for a webhook-triggered deploy.
type WebhookApproval struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	WebhookLogID string   `json:"webhook_log_id"`
	Branch      string    `json:"branch"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	EventType   string    `json:"event_type"` // push, tag, pull_request
	TriggeredBy string    `json:"triggered_by"` // webhook_auto_deploy
	Status      string    `json:"status"` // pending, approved, rejected
	ApproverID  string    `json:"approver_id,omitempty"` // user who approved/rejected
	ApprovedAt  *time.Time `json:"approved_at,omitempty"`
	RejectedAt  *time.Time `json:"rejected_at,omitempty"`
	Reason      string    `json:"reason,omitempty"` // rejection reason if rejected
	CreatedAt   time.Time `json:"created_at"`
}