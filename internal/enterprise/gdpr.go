package enterprise

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// GDPRExporter handles GDPR compliance — data export and right to erasure.
type GDPRExporter struct {
	store core.Store
}

// NewGDPRExporter creates a GDPR compliance handler.
func NewGDPRExporter(store core.Store) *GDPRExporter {
	return &GDPRExporter{store: store}
}

// ExportData collects all personal data for a user (GDPR Article 15/20).
func (g *GDPRExporter) ExportData(ctx context.Context, userID string) (*DataExport, error) {
	user, err := g.store.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	membership, _ := g.store.GetUserMembership(ctx, userID)

	export := &DataExport{
		ExportedAt: time.Now(),
		User: UserData{
			ID:        user.ID,
			Email:     user.Email,
			Name:      user.Name,
			AvatarURL: user.AvatarURL,
			Status:    user.Status,
			CreatedAt: user.CreatedAt,
		},
	}

	if membership != nil {
		export.TenantID = membership.TenantID
		export.RoleID = membership.RoleID

		// Export user's apps
		apps, _, _ := g.store.ListAppsByTenant(ctx, membership.TenantID, 1000, 0)
		export.Applications = len(apps)

		// Export audit logs
		logs, _, _ := g.store.ListAuditLogs(ctx, membership.TenantID, 1000, 0)
		export.AuditEntries = len(logs)
	}

	return export, nil
}

// EraseData removes all personal data for a user (GDPR Article 17).
// Anonymizes records that must be retained for legal reasons.
func (g *GDPRExporter) EraseData(ctx context.Context, userID string) error {
	user, err := g.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	// Anonymize user data
	user.Email = fmt.Sprintf("deleted-%s@anonymized.local", user.ID[:8])
	user.Name = "Deleted User"
	user.AvatarURL = ""
	user.PasswordHash = ""
	user.Status = "deleted"

	return g.store.UpdateUser(ctx, user)
}

// DataExport holds all exportable personal data.
type DataExport struct {
	ExportedAt   time.Time `json:"exported_at"`
	User         UserData  `json:"user"`
	TenantID     string    `json:"tenant_id"`
	RoleID       string    `json:"role_id"`
	Applications int       `json:"applications_count"`
	AuditEntries int       `json:"audit_entries_count"`
}

// UserData is the exportable subset of user information.
type UserData struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ToJSON serializes the export to JSON.
func (d *DataExport) ToJSON() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}
