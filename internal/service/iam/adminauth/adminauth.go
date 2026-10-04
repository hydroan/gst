package adminauth

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/consts"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/tenant"
)

// EnsureTenantAdmin verifies admin-user operations inside the current tenant.
//
// The helper is shared by the admin user, admin session, and password reset
// flows, and reaches other modules through iam.EnsureAdminOnUser. System-root
// actors bypass tenant checks. Tenant administrators must pass route
// authorization in the current tenant, and when a concrete target is supplied
// the target must also be a member of that tenant. System-root targets are
// never manageable through tenant-local admin APIs.
func EnsureTenantAdmin(ctx *gst.ServiceContext, actor *modeliamuser.User, target *modeliamuser.User) error {
	systemRootActor, err := isSystemRoot(ctx, actor)
	if err != nil {
		return gst.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if systemRootActor {
		return nil
	}
	if actor == nil || actor.GetID() == "" {
		return gst.NewError(http.StatusForbidden, "permission denied")
	}

	// Root may appear in tenant RBAC bindings for setup or bootstrap purposes,
	// but tenant-local administrators must not manage root as a target user.
	systemRootTarget, err := isSystemRoot(ctx, target)
	if err != nil {
		return gst.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if systemRootTarget {
		return gst.NewError(http.StatusForbidden, "permission denied")
	}

	tenant := currentTenant(ctx)
	// Route permission and target membership are checked separately. A user can
	// have permission to call the endpoint without being allowed to manage a
	// particular target outside the current tenant.
	decision, err := rbac.RBAC().Authorize(ctx, tenant, actor.GetID(), operationObject(ctx), operationAction(ctx))
	if err != nil {
		return gst.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if !decision.Allowed {
		return gst.NewError(http.StatusForbidden, "permission denied")
	}

	if target == nil {
		return nil
	}
	belongs, err := targetBelongsToTenant(ctx, tenant, target.GetID())
	if err != nil {
		return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to verify target tenant", err)
	}
	if !belongs {
		return gst.NewError(http.StatusForbidden, "target user is outside tenant")
	}
	return nil
}

// currentTenant returns the authorization domain for an admin request.
//
// Tenant middleware writes TenantID into ServiceContext. If no tenant resolver is
// installed, admin APIs operate in the default authorization domain.
func currentTenant(ctx *gst.ServiceContext) string {
	if ctx != nil && strings.TrimSpace(ctx.TenantID()) != "" {
		return strings.TrimSpace(ctx.TenantID())
	}
	return tenant.Default
}

// routeParam matches a parameter of a route as the router registers it,
// :id in /api/iam/admin/users/:id.
var routeParam = regexp.MustCompile(`:([a-zA-Z0-9_]+)`)

// operationObject returns the object of the RBAC decision: the route of the
// action with the request's parameters filled in, /api/iam/admin/users/42,
// the same on both transports, where the path of an HTTP request is that
// and the path of a gRPC call names the rpc, which no policy names. The
// path stands in when the context carries no route, or when the route
// names a parameter the request does not carry.
func operationObject(ctx *gst.ServiceContext) string {
	if ctx == nil {
		return ""
	}
	route := strings.TrimSpace(ctx.Route())
	if route == "" {
		return strings.TrimSpace(ctx.Path())
	}
	filled := true
	object := routeParam.ReplaceAllStringFunc(route, func(param string) string {
		value := ctx.Param(strings.TrimPrefix(param, ":"))
		if value == "" {
			filled = false
		}
		return value
	})
	if !filled {
		return strings.TrimSpace(ctx.Path())
	}
	return object
}

// operationAction returns the action string used for RBAC route authorization.
func operationAction(ctx *gst.ServiceContext) string {
	if ctx == nil {
		return ""
	}
	return strings.TrimSpace(ctx.Method())
}

// targetBelongsToTenant reports whether the target has any role binding in tenant.
//
// User rows do not carry tenant_id, so target visibility is derived from RBAC
// role bindings rather than from the IAM user table.
func targetBelongsToTenant(ctx *gst.ServiceContext, tenant string, userID string) (bool, error) {
	if strings.TrimSpace(userID) == "" {
		return false, nil
	}
	roles, err := rbac.RBAC().RolesForSubject(ctx, tenant, userID)
	if err != nil {
		return false, err
	}
	return len(roles) > 0, nil
}

// isSystemRoot reports whether user holds the framework-level root role.
func isSystemRoot(ctx *gst.ServiceContext, user *modeliamuser.User) (bool, error) {
	if user == nil || strings.TrimSpace(user.GetID()) == "" {
		return false, nil
	}
	return rbac.RBAC().HasSystemRole(ctx, user.GetID(), consts.AUTHZ_SYSTEM_ROLE_ROOT)
}
