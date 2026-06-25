package serviceauthz

import (
	gstdao "github.com/hydroan/gst/dao"
	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

type UserRoleService struct {
	service.Base[*modelauthz.UserRole, *modelauthz.UserRole, *modelauthz.UserRole]
}

// DeleteAfter support filter and delete multiple user_roles by query parameter `username` and `rolecode`.
func (s *UserRoleService) DeleteAfter(ctx *types.ServiceContext, userRole *modelauthz.UserRole) error {
	log := s.WithServiceContext(ctx, consts.PHASE_DELETE_AFTER)
	username := ctx.Query().Get("username")
	roleCode := ctx.Query().Get("rolecode")

	userRoles := make([]*modelauthz.UserRole, 0)
	if err := database.Database[*modelauthz.UserRole](ctx).WithQuery(&modelauthz.UserRole{Username: username, RoleCode: roleCode}).List(&userRoles); err != nil {
		log.Error(err)
		return err
	}
	for _, rb := range userRoles {
		log.Infoz("will delete user role", zap.Object("user_role", rb))
	}
	if err := database.Database[*modelauthz.UserRole](ctx).WithPurge().Delete(userRoles...); err != nil {
		return err
	}

	return nil
}

func (s *UserRoleService) ListAfter(ctx *types.ServiceContext, data *[]*modelauthz.UserRole) error {
	log := s.WithServiceContext(ctx, consts.PHASE_LIST_AFTER)

	userMap, err := gstdao.QueryModelsMap(ctx, func(u *modelauthz.User) string { return u.ID }, nil)
	if err != nil {
		log.Error(err)
		return err
	}
	roleMap, err := gstdao.QueryModelsMap(ctx, func(r *modelauthz.Role) string { return r.ID }, nil)
	if err != nil {
		log.Error(err)
		return err
	}

	for _, ur := range *data {
		ur.User = userMap[ur.UserID]
		ur.Role = roleMap[ur.RoleID]
	}

	return nil
}
