package serviceiamuser

import (
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// LoadActor resolves the authenticated user for the current request.
//
// IAM admin APIs use the session snapshot only as the authentication source;
// they still reload the user row from the database so status, IDs, and system
// role checks operate on the current persisted user.
func LoadActor(ctx *types.ServiceContext) (*modeliamuser.User, error) {
	_, session, err := serviceiamsession.SessionManager.Current(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "invalid session")
	}
	if session.UserID == "" {
		return nil, service.NewError(http.StatusUnauthorized, "current user not found")
	}

	actor := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(actor, session.UserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, service.NewError(http.StatusUnauthorized, "current user not found")
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load current user", err)
	}
	return actor, nil
}

// LoadActorAndTarget resolves the actor and target users used by admin actions.
//
// It intentionally does not authorize the relationship between the two users.
// Callers must pass both users to adminauth.EnsureTenantAdmin so target-specific
// rules, such as tenant membership and system-root protection, are evaluated in
// one place.
func LoadActorAndTarget(ctx *types.ServiceContext, targetUserID string) (*modeliamuser.User, *modeliamuser.User, error) {
	actor, err := LoadActor(ctx)
	if err != nil {
		return nil, nil, err
	}

	target := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx).Get(target, targetUserID); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, nil, service.NewError(http.StatusNotFound, "user not found")
		}
		return nil, nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load target user", err)
	}

	return actor, target, nil
}

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
	return rbac.DefaultTenant
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
	return rbac.RBAC().HasSystemRole(ctx, actor.GetID(), consts.AUTHZ_SYSTEM_ROLE_ROOT)
}
