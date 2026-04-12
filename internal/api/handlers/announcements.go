package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AnnouncementHandler manages platform-wide announcements.
type AnnouncementHandler struct {
	bolt core.BoltStorer
}

func NewAnnouncementHandler(bolt core.BoltStorer) *AnnouncementHandler {
	return &AnnouncementHandler{bolt: bolt}
}

// Announcement is a platform-wide broadcast message.
type Announcement struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Type      string     `json:"type"` // info, warning, critical, maintenance
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// announcementList wraps the persisted list of announcements.
type announcementList struct {
	Items []Announcement `json:"items"`
}

// List handles GET /api/v1/announcements
// Returns active announcements for the dashboard banner.
func (h *AnnouncementHandler) List(w http.ResponseWriter, _ *http.Request) {
	var list announcementList
	_ = h.bolt.Get("announcements", "all", &list)

	active := make([]Announcement, 0)
	now := time.Now()
	for _, a := range list.Items {
		if a.Active && (a.ExpiresAt == nil || a.ExpiresAt.After(now)) {
			active = append(active, a)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": active, "total": len(active)})
}

// Create handles POST /api/v1/admin/announcements. Authorized by
// middleware.RequireSuperAdmin at the router.
func (h *AnnouncementHandler) Create(w http.ResponseWriter, r *http.Request) {
	var a Announcement
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(a.Title) > 200 {
		writeError(w, http.StatusBadRequest, "title must be 200 characters or less")
		return
	}
	if len(a.Body) > 10000 {
		writeError(w, http.StatusBadRequest, "body must be 10000 characters or less")
		return
	}

	a.ID = core.GenerateID()
	a.Active = true
	a.CreatedAt = time.Now()

	var list announcementList
	_ = h.bolt.Get("announcements", "all", &list)

	if len(list.Items) >= 100 {
		writeError(w, http.StatusConflict, "announcement limit reached (100)")
		return
	}
	list.Items = append(list.Items, a)

	if err := h.bolt.Set("announcements", "all", list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save announcement")
		return
	}

	writeJSON(w, http.StatusCreated, a)
}

// Dismiss handles DELETE /api/v1/admin/announcements/{id}
func (h *AnnouncementHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	var list announcementList
	if err := h.bolt.Get("announcements", "all", &list); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	for i := range list.Items {
		if list.Items[i].ID == id {
			list.Items[i].Active = false
			break
		}
	}

	if err := h.bolt.Set("announcements", "all", list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update announcement")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
