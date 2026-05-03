package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TeamHandler handles team management endpoints.
type TeamHandler struct {
	store  core.Store
	events *core.EventBus
}

type teamMemberStore interface {
	ListTeamMembers(ctx context.Context, tenantID string) ([]core.TeamMember, error)
	RemoveTeamMember(ctx context.Context, tenantID, memberID string) error
}

func NewTeamHandler(store core.Store, events *core.EventBus) *TeamHandler {
	return &TeamHandler{store: store, events: events}
}

// TeamMemberView is the UI-facing view for tenant members.
type TeamMemberView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	JoinedAt  time.Time `json:"joined_at"`
}

// ListMembers handles GET /api/v1/team/members.
func (h *TeamHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if memberStore, ok := h.store.(teamMemberStore); ok {
		members, err := memberStore.ListTeamMembers(r.Context(), claims.TenantID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load members")
			return
		}

		views := make([]TeamMemberView, 0, len(members))
		for _, member := range members {
			user, err := h.store.GetUser(r.Context(), member.UserID)
			if err != nil {
				if err == core.ErrNotFound {
					continue
				}
				writeError(w, http.StatusInternalServerError, "failed to load member user")
				return
			}
			views = append(views, TeamMemberView{
				ID:        member.ID,
				Name:      user.Name,
				Email:     user.Email,
				Role:      member.RoleID,
				AvatarURL: user.AvatarURL,
				JoinedAt:  member.CreatedAt,
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"data":  views,
			"total": len(views),
		})
		return
	}

	user, err := h.store.GetUser(r.Context(), claims.UserID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "user not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to load user")
		}
		return
	}

	member, err := h.store.GetUserMembership(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load membership")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": []TeamMemberView{{
			ID:        member.ID,
			Name:      user.Name,
			Email:     user.Email,
			Role:      member.RoleID,
			AvatarURL: user.AvatarURL,
			JoinedAt:  member.CreatedAt,
		}},
		"total": 1,
	})
}

// RemoveMember handles DELETE /api/v1/team/members/{id}.
func (h *TeamHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	memberID := r.PathValue("id")
	if memberID == "" {
		writeError(w, http.StatusBadRequest, "member id is required")
		return
	}

	memberStore, ok := h.store.(teamMemberStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "team member removal is not supported by this store")
		return
	}

	members, err := memberStore.ListTeamMembers(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load members")
		return
	}

	var target *core.TeamMember
	for i := range members {
		if members[i].ID == memberID {
			target = &members[i]
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	if target.UserID == claims.UserID {
		writeError(w, http.StatusBadRequest, "cannot remove yourself")
		return
	}

	if err := memberStore.RemoveTeamMember(r.Context(), claims.TenantID, memberID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "member not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to remove member")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ListRoles handles GET /api/v1/team/roles
func (h *TeamHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	roles, err := h.store.ListRoles(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": roles, "total": len(roles)})
}

// GetAuditLog handles GET /api/v1/team/audit-log
func (h *TeamHandler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	pg := parsePagination(r)

	entries, total, err := h.store.ListAuditLogs(r.Context(), claims.TenantID, pg.PerPage, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writePaginatedJSON(w, entries, total, pg)
}
