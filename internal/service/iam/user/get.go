package serviceiamuser

import (
	"net/http"

	"github.com/hydroan/gst/internal/service/iam/adminauth"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"

	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
)

// AdminUserGetService handles GET /iam/admin/users/:id for privileged administrators.
//
// The endpoint returns an IAM admin view only after the shared tenant-admin
// authorization helper has validated both the actor's endpoint permission and
// the target user's tenant visibility.
type AdminUserGetService struct {
	service.Base[*modeliamuser.User, *model.Empty, *modeliamuser.AdminUserGetRsp]
}

// Get returns one user visible to the current administrator.
func (a *AdminUserGetService) Get(ctx *types.ServiceContext, req *model.Empty) (rsp *modeliamuser.AdminUserGetRsp, err error) {
	log := a.WithContext(ctx, ctx.Phase())

	targetUserID := ctx.Param("id")
	if targetUserID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user id is required")
	}

	// Load the actor and target before authorization so EnsureTenantAdmin can
	// reject system-root targets and verify tenant membership using the concrete
	// target user ID.
	actor, target, err := LoadActorAndTarget(ctx, targetUserID)
	if err != nil {
		log.Error("failed to resolve actor or target user", err)
		return nil, err
	}
	// EnsureTenantAdmin grants system-root actors globally. For tenant admins it
	// first checks route permission in the current tenant, then checks that the
	// target user has a role binding in that same tenant.
	if err = adminauth.EnsureTenantAdmin(ctx, actor, target); err != nil {
		log.Error("admin user get denied", err)
		return nil, err
	}

	view, err := buildAdminUserView(ctx, target)
	if err != nil {
		log.Error("failed to build admin user view", err)
		return nil, err
	}
	return &modeliamuser.AdminUserGetRsp{User: view}, nil
}
