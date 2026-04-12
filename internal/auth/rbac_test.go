package auth

import (
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

func TestHasPermission_SuperAdmin(t *testing.T) {
	if !HasPermission("role_super_admin", "anything.at.all") {
		t.Error("super admin should have all permissions")
	}
}

func TestHasPermission_ExactMatch(t *testing.T) {
	SetRoleInCache(&models.Role{
		ID:              "role_dev",
		PermissionsJSON: `["app.view","app.deploy"]`,
	})

	if !HasPermission("role_dev", "app.view") {
		t.Error("should have app.view")
	}
	if !HasPermission("role_dev", "app.deploy") {
		t.Error("should have app.deploy")
	}
	if HasPermission("role_dev", "app.delete") {
		t.Error("should NOT have app.delete")
	}
}

func TestHasPermission_WildcardMatch(t *testing.T) {
	SetRoleInCache(&models.Role{
		ID:              "role_owner",
		PermissionsJSON: `["app.*","project.view"]`,
	})

	if !HasPermission("role_owner", "app.deploy") {
		t.Error("app.* should match app.deploy")
	}
	if !HasPermission("role_owner", "app.delete") {
		t.Error("app.* should match app.delete")
	}
	if !HasPermission("role_owner", "project.view") {
		t.Error("should have exact project.view")
	}
	if HasPermission("role_owner", "project.delete") {
		t.Error("should NOT have project.delete")
	}
}

func TestHasPermission_StarAll(t *testing.T) {
	SetRoleInCache(&models.Role{
		ID:              "role_god",
		PermissionsJSON: `["*"]`,
	})

	if !HasPermission("role_god", "anything") {
		t.Error("* should match everything")
	}
}

func TestHasPermission_UnknownRole(t *testing.T) {
	if HasPermission("role_nonexistent", "app.view") {
		t.Error("unknown role should have no permissions")
	}
}
