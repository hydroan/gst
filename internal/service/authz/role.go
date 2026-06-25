package serviceauthz

import (
	"fmt"
	"strings"

	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

type RoleService struct {
	service.Base[*modelauthz.Role, *modelauthz.Role, *modelauthz.Role]
}

// DeleteAfter support filter and delete multiple roles by query parameter `name`.
func (r *RoleService) DeleteAfter(ctx *types.ServiceContext, role *modelauthz.Role) error {
	log := r.WithServiceContext(ctx, consts.PHASE_DELETE_AFTER)
	name := ctx.URL().Query().Get("name")
	if len(name) == 0 {
		return nil
	}

	roles := make([]*modelauthz.Role, 0)
	if err := database.Database[*modelauthz.Role](ctx).WithQuery(&modelauthz.Role{Name: name}).List(&roles); err != nil {
		log.Error(err)
		return err
	}
	for _, role := range roles {
		log.Infoz("will delete role", zap.Object("role", role))
	}
	if err := database.Database[*modelauthz.Role](ctx).WithPurge().Delete(roles...); err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func (r *RoleService) CreateAfter(ctx *types.ServiceContext, role *modelauthz.Role) error {
	return r.remarkMenus(ctx, role)
}

func (r *RoleService) UpdateAfter(ctx *types.ServiceContext, role *modelauthz.Role) error {
	return r.remarkMenus(ctx, role)
}

func (r *RoleService) PatchAfter(ctx *types.ServiceContext, role *modelauthz.Role) error {
	return r.remarkMenus(ctx, role)
}

// remarkMenus remark role about menus
func (r *RoleService) remarkMenus(ctx *types.ServiceContext, role *modelauthz.Role) error {
	log := r.WithServiceContext(ctx, ctx.GetPhase())

	menus := make([]*modelauthz.Menu, 0)
	if err := database.Database[*modelauthz.Menu](ctx).List(&menus); err != nil {
		log.Error(err)
		return err
	}

	menuMap := make(map[string]*modelauthz.Menu)
	for _, m := range menus {
		menuMap[m.ID] = m
	}

	var sb strings.Builder
	if len(role.MenuPartialIDs) > 0 {
		sb.WriteString("父菜单\n")
	}
	for _, mid := range role.MenuPartialIDs {
		if menu, ok := menuMap[mid]; ok {
			fmt.Fprintf(&sb, "    %s\n", menu.Label)
		}
	}
	if len(role.MenuIDs) > 0 {
		sb.WriteString("\n子菜单\n")
	}
	for _, mid := range role.MenuIDs {
		if menu, ok := menuMap[mid]; ok {
			fmt.Fprintf(&sb, "    %s\n", menu.Label)
		}
	}

	role.Remark = new(strings.TrimSpace(sb.String()))

	// NOTE: Role has "UpdateBefore" hook to update role's permissions.
	// this service operations just update role's remark, so we should not invoke any "hooks" here.
	if err := database.Database[*modelauthz.Role](ctx).WithoutHook().Update(role); err != nil {
		log.Error(err)
		return err
	}

	log.Info("update remark about menus successfully")

	return nil
}
