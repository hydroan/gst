package adminauth

import (
	"net/http"
	"strings"

	"github.com/hydroan/gst/authz/rbac"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// EnsureTenantAdmin verifies admin-user operations inside the current tenant.
//
// The helper is shared by user list/get/status flows. System-root actors bypass
// tenant checks. Tenant administrators must pass route authorization in the
// current tenant, and when a concrete target is supplied the target must also be
// a member of that tenant. System-root targets are never manageable through
// tenant-local admin APIs.
func EnsureTenantAdmin(ctx *types.ServiceContext, actor *modeliamuser.User, target *modeliamuser.User) error {
	systemRootActor, err := isSystemRoot(ctx, actor)
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if systemRootActor {
		return nil
	}
	if actor == nil || actor.GetID() == "" {
		return service.NewError(http.StatusForbidden, "permission denied")
	}

	// Root may appear in tenant RBAC bindings for setup or bootstrap purposes,
	// but tenant-local administrators must not manage root as a target user.
	systemRootTarget, err := isSystemRoot(ctx, target)
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if systemRootTarget {
		return service.NewError(http.StatusForbidden, "permission denied")
	}

	tenant := currentTenant(ctx)
	// Route permission and target membership are checked separately. A user can
	// have permission to call the endpoint without being allowed to manage a
	// particular target outside the current tenant.
	allowed, err := rbac.RBAC().Authorize(ctx, tenant, actor.GetID(), operationObject(ctx), operationAction(ctx))
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if !allowed {
		return service.NewError(http.StatusForbidden, "permission denied")
	}

	if target == nil {
		return nil
	}
	belongs, err := targetBelongsToTenant(ctx, tenant, target.GetID())
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify target tenant", err)
	}
	if !belongs {
		return service.NewError(http.StatusForbidden, "target user is outside tenant")
	}
	return nil
}

// currentTenant returns the authorization domain for an admin request.
//
// Tenant middleware writes TenantID into ServiceContext. If no tenant resolver is
// installed, admin APIs operate in the default authorization domain.
func currentTenant(ctx *types.ServiceContext) string {
	if ctx != nil && strings.TrimSpace(ctx.TenantID()) != "" {
		return strings.TrimSpace(ctx.TenantID())
	}
	return rbac.DefaultTenant
}

// operationObject returns the object string used for RBAC route authorization.
//
// ServiceContext.Path contains the concrete request path in normal HTTP flows.
// Route is kept as a fallback for service-level tests or callers that construct
// contexts without an HTTP request.
func operationObject(ctx *types.ServiceContext) string {
	if ctx == nil {
		return ""
	}
	if path := strings.TrimSpace(ctx.Path()); path != "" {
		return path
	}
	return strings.TrimSpace(ctx.Route())
}

// operationAction returns the action string used for RBAC route authorization.
func operationAction(ctx *types.ServiceContext) string {
	if ctx == nil {
		return ""
	}
	return strings.TrimSpace(ctx.Method())
}

// targetBelongsToTenant reports whether the target has any role binding in tenant.
//
// User rows do not carry tenant_id, so target visibility is derived from RBAC
// role bindings rather than from the IAM user table.
func targetBelongsToTenant(ctx *types.ServiceContext, tenant string, userID string) (bool, error) {
	if strings.TrimSpace(userID) == "" {
		return false, nil
	}
	return rbac.RBAC().SubjectInTenant(ctx, tenant, userID)
}

// isSystemRoot reports whether user holds the framework-level root role.
func isSystemRoot(ctx *types.ServiceContext, user *modeliamuser.User) (bool, error) {
	if user == nil || strings.TrimSpace(user.GetID()) == "" {
		return false, nil
	}
	return rbac.RBAC().HasSystemRole(ctx, user.GetID(), consts.AUTHZ_SYSTEM_ROLE_ROOT)
}
