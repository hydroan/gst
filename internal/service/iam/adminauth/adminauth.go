package adminauth

import (
	"net/http"
	"strings"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// EnsureTenantAdmin verifies that actor may manage target inside the current tenant.
func EnsureTenantAdmin(ctx *types.ServiceContext, actor *modeliamuser.User, target *modeliamuser.User) error {
	systemRootActor, err := isSystemRoot(actor)
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if systemRootActor {
		return nil
	}
	if actor == nil || actor.GetID() == "" {
		return service.NewError(http.StatusForbidden, "permission denied")
	}
	systemRootTarget, err := isSystemRoot(target)
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if systemRootTarget {
		return service.NewError(http.StatusForbidden, "permission denied")
	}

	tenant := currentTenant(ctx)
	allowed, err := rbac.RBAC().Authorize(tenant, actor.GetID(), operationObject(ctx), operationAction(ctx))
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

func currentTenant(ctx *types.ServiceContext) string {
	if ctx != nil && strings.TrimSpace(ctx.TenantID()) != "" {
		return strings.TrimSpace(ctx.TenantID())
	}
	return rbac.DefaultTenant
}

func operationObject(ctx *types.ServiceContext) string {
	if ctx == nil {
		return ""
	}
	if path := strings.TrimSpace(ctx.Path()); path != "" {
		return path
	}
	return strings.TrimSpace(ctx.Route())
}

func operationAction(ctx *types.ServiceContext) string {
	if ctx == nil {
		return ""
	}
	return strings.TrimSpace(ctx.Method())
}

func targetBelongsToTenant(ctx *types.ServiceContext, tenant string, userID string) (bool, error) {
	if strings.TrimSpace(userID) == "" {
		return false, nil
	}

	roleBindings := make([]*modelauthz.RoleBinding, 0, 1)
	if err := database.Database[*modelauthz.RoleBinding](ctx).
		WithLimit(1).
		WithQuery(&modelauthz.RoleBinding{TenantID: tenant, SubjectID: userID}).
		List(&roleBindings); err != nil {
		return false, err
	}
	return len(roleBindings) > 0, nil
}

func isSystemRoot(user *modeliamuser.User) (bool, error) {
	if user == nil || strings.TrimSpace(user.GetID()) == "" {
		return false, nil
	}
	return rbac.RBAC().HasSystemRole(user.GetID(), consts.AUTHZ_SYSTEM_ROLE_ROOT)
}
