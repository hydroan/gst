package serviceiamgroup

import (
	"net/http"

	"github.com/hydroan/gst/database"
	modeliamgroup "github.com/hydroan/gst/internal/model/iam/group"
	modeliamtenant "github.com/hydroan/gst/internal/model/iam/tenant"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/response"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// GroupService handles CRUD operations for IAM groups.
type GroupService struct {
	service.Base[*modeliamgroup.Group, *modeliamgroup.Group, *modeliamgroup.Group]
}

func (GroupService) CreateBefore(ctx *types.ServiceContext, req *modeliamgroup.Group) error {
	if err := ensureGroupModuleSuperuser(ctx); err != nil {
		return err
	}
	return ensureGroupTenantAccessible(ctx, req)
}

func (GroupService) DeleteBefore(ctx *types.ServiceContext, _ *modeliamgroup.Group) error {
	return ensureGroupModuleSuperuser(ctx)
}

func (GroupService) UpdateBefore(ctx *types.ServiceContext, req *modeliamgroup.Group) error {
	if err := ensureGroupModuleSuperuser(ctx); err != nil {
		return err
	}
	return ensureGroupTenantAccessible(ctx, req)
}

func (GroupService) PatchBefore(ctx *types.ServiceContext, req *modeliamgroup.Group) error {
	if err := ensureGroupModuleSuperuser(ctx); err != nil {
		return err
	}
	return ensureGroupTenantAccessible(ctx, req)
}

func (GroupService) ListBefore(ctx *types.ServiceContext, _ *[]*modeliamgroup.Group) error {
	return ensureGroupModuleSuperuser(ctx)
}

func (GroupService) GetBefore(ctx *types.ServiceContext, _ *modeliamgroup.Group) error {
	return ensureGroupModuleSuperuser(ctx)
}

func (GroupService) CreateManyBefore(ctx *types.ServiceContext, groups ...*modeliamgroup.Group) error {
	if err := ensureGroupModuleSuperuser(ctx); err != nil {
		return err
	}
	return ensureGroupTenantsAccessible(ctx, groups...)
}

func (GroupService) DeleteManyBefore(ctx *types.ServiceContext, _ ...*modeliamgroup.Group) error {
	return ensureGroupModuleSuperuser(ctx)
}

func (GroupService) UpdateManyBefore(ctx *types.ServiceContext, groups ...*modeliamgroup.Group) error {
	if err := ensureGroupModuleSuperuser(ctx); err != nil {
		return err
	}
	return ensureGroupTenantsAccessible(ctx, groups...)
}

func (GroupService) PatchManyBefore(ctx *types.ServiceContext, groups ...*modeliamgroup.Group) error {
	if err := ensureGroupModuleSuperuser(ctx); err != nil {
		return err
	}
	return ensureGroupTenantsAccessible(ctx, groups...)
}

func ensureGroupModuleSuperuser(ctx *types.ServiceContext) error {
	_, session, err := serviceiamsession.GetCurrentSession(ctx)
	if err != nil {
		return err
	}

	actor := new(modeliamuser.User)
	if err = database.Database[*modeliamuser.User](ctx.DatabaseContext()).Get(actor, session.UserID); err != nil {
		return types.NewServiceErrorWithCause(http.StatusUnauthorized, "current user not found", err)
	}
	if actor.ID == "" {
		return types.NewServiceError(http.StatusUnauthorized, "current user not found")
	}
	if actor.Username == consts.AUTHZ_USER_ROOT || actor.Username == consts.AUTHZ_USER_ADMIN {
		return nil
	}
	if actor.IsSuperuser != nil && *actor.IsSuperuser {
		return nil
	}
	return types.NewServiceError(http.StatusForbidden, "forbidden: superuser privileges required", response.CodeForbidden)
}

func ensureGroupTenantsAccessible(ctx *types.ServiceContext, groups ...*modeliamgroup.Group) error {
	for _, group := range groups {
		if err := ensureGroupTenantAccessible(ctx, group); err != nil {
			return err
		}
	}
	return nil
}

func ensureGroupTenantAccessible(ctx *types.ServiceContext, group *modeliamgroup.Group) error {
	if group == nil || group.TenantID == nil || *group.TenantID == "" {
		return nil
	}
	tenant := new(modeliamtenant.Tenant)
	if err := database.Database[*modeliamtenant.Tenant](ctx.DatabaseContext()).Get(tenant, *group.TenantID); err != nil {
		return types.NewServiceErrorWithCause(http.StatusInternalServerError, "failed to load target tenant", err)
	}
	if tenant.ID == "" {
		return types.NewServiceError(http.StatusNotFound, "tenant not found")
	}
	return nil
}
