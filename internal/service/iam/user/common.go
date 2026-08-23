package serviceiamuser

import (
	"net/http"
	"strings"

	"github.com/hydroan/gst/authz/rbac"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// shouldInvalidateUserSessions returns whether a user status transition must revoke all active sessions.
func shouldInvalidateUserSessions(status modeliamuser.UserStatus) bool {
	return status == modeliamuser.UserStatusInactive || status == modeliamuser.UserStatusLocked
}

// currentTenant returns the authorization domain used by admin user list scopes.
//
// Tenant-aware applications populate TenantID through middleware. Applications
// without tenant middleware operate in the default authorization domain.
func currentTenant(ctx *types.ServiceContext) string {
	if ctx != nil && strings.TrimSpace(ctx.TenantID()) != "" {
		return strings.TrimSpace(ctx.TenantID())
	}
	return tenant.Default
}

// isSystemRoot reports whether actor has the framework-level root role.
//
// System root is intentionally separate from tenant-local roles: it can bypass
// tenant list scoping as an actor, and tenant admins must not manage it as a
// target even if root also has tenant role bindings.
func isSystemRoot(ctx *types.ServiceContext, actor *modeliamuser.User) (bool, error) {
	if actor == nil || strings.TrimSpace(actor.GetID()) == "" {
		return false, nil
	}
	systemRoot, err := rbac.RBAC().HasSystemRole(ctx, actor.GetID(), consts.AUTHZ_SYSTEM_ROLE_ROOT)
	if err != nil {
		return false, service.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	return systemRoot, nil
}
