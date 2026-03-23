package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AnnouncementHandler manages platform-wide announcements.
type AnnouncementHandler struct {
	mu            sync.RWMutex
	announcements []Announcement
}

func NewAnnouncementHandler() *AnnouncementHandler {
	return &AnnouncementHandler{}
}

// Announcement is a platform-wide broadcast message.
type Announcement struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Type      string    `json:"type"` // info, warning, critical, maintenance
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// List handles GET /api/v1/announcements
// Returns active announcements for the dashboard banner.
func (h *AnnouncementHandler) List(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	active := make([]Announcement, 0)
	now := time.Now()
	for _, a := range h.announcements {
		if a.Active && (a.ExpiresAt == nil || a.ExpiresAt.After(now)) {
			active = append(active, a)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": active, "total": len(active)})
}

// Create handles POST /api/v1/admin/announcements
func (h *AnnouncementHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.RoleID != "role_super_admin" {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}

	var a Announcement
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	a.ID = core.GenerateID()
	a.Active = true
	a.CreatedAt = time.Now()

	h.mu.Lock()
	h.announcements = append(h.announcements, a)
	h.mu.Unlock()

	writeJSON(w, http.StatusCreated, a)
}

// Dismiss handles DELETE /api/v1/admin/announcements/{id}
func (h *AnnouncementHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	h.mu.Lock()
	defer h.mu.Unlock()

	for i := range h.announcements {
		if h.announcements[i].ID == id {
			h.announcements[i].Active = false
			break
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
