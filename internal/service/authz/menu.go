package serviceauthz

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/samber/lo"
)

type MenuService struct {
	service.Base[*modelauthz.Menu, *modelauthz.Menu, *modelauthz.Menu]
}

func (m *MenuService) ListAfter(ctx *types.ServiceContext, data *[]*modelauthz.Menu) error {
	return m.filterByRole(ctx, data, m.WithContext(ctx, ctx.Phase()))
}

func (m *MenuService) filterByRole(ctx *types.ServiceContext, data *[]*modelauthz.Menu, log types.Logger) error {
	// The built-in root account is identified by the stable user ID.
	if ctx.UserID() == consts.AUTHZ_USER_ROOT {
		return nil
	}

	var (
		user      = new(modelauthz.User)
		userRoles = make([]*modelauthz.UserRole, 0)
		roles     = make([]*modelauthz.Role, 0)
	)

	// query the current user
	if err := database.Database[*modelauthz.User](ctx).Get(user, ctx.UserID()); err != nil {
		log.Error(err)
		return err
	}

	// query all "UserRole" according to the current user id.
	if err := database.Database[*modelauthz.UserRole](ctx).
		WithQuery(&modelauthz.UserRole{UserID: ctx.UserID()}).
		List(&userRoles); err != nil {
		log.Error(err)
		return err
	}

	// query all "Role" according to the "UserRole"
	if len(userRoles) > 0 {
		roleIDs := make([]string, 0)
		for _, ur := range userRoles {
			if len(ur.RoleID) > 0 {
				roleIDs = append(roleIDs, ur.RoleID)
			}
		}
		if err := database.Database[*modelauthz.Role](ctx).
			WithQuery(&modelauthz.Role{Base: model.Base{ID: strings.Join(roleIDs, ",")}}).List(&roles); err != nil {
			log.Error(err)
			return err
		}
	}
	// the user has no roles, use the default role.
	if len(roles) == 0 {
		if err := database.Database[*modelauthz.Role](ctx).
			WithQuery(&modelauthz.Role{Default: new(true)}).
			List(&roles); err != nil {
			log.Error(err)
			return err
		}
	}
	if len(roles) == 0 {
		log.Warn("user has no roles and don't have default role")
		// Clear the slice by dereferencing the pointer and assigning a new empty slice
		*data = make([]*modelauthz.Menu, 0)
		return nil
	}
	for _, r := range roles {
		log.Infow("role", "username", ctx.Username(), "role_name", r.Name, "role_code", r.Code)
	}

	{
		menuMap := make(map[string]struct{})
		for _, role := range roles {
			for _, id := range role.MenuIDs {
				menuMap[id] = struct{}{}
			}
			// 这里需要把 MenuPartialIds 加进去, 父菜单下面有多个菜单, 如果只选中了部分, 则是将 id 放在 MenuPartialIds.
			for _, id := range role.MenuPartialIDs {
				menuMap[id] = struct{}{}
			}
		}
		// fmt.Println("---- menuMap", len(menuMap))

		_data := lo.Filter[*modelauthz.Menu](*data, func(item *modelauthz.Menu, _ int) bool {
			var exists, matched, ok bool
			_, exists = menuMap[item.ID]
			if exists {
				if matched, _ = regexp.MatchString(item.DomainPattern, ctx.Host()); matched {
					ok = true
				}
			}
			return ok
			// if _, ok := menuMap[item.ID]; ok {
			// 	return true
			// } else {
			// 	return false
			// }
		})
		for i := range _data {
			filter(ctx, _data[i], menuMap)
		}
		val := reflect.ValueOf(data)
		val.Elem().Set(reflect.ValueOf(_data))
		return nil
	}
}

// 递归过滤出当前角色所拥有的菜单. 作用于 menu.Children 字段.
func filter(ctx *types.ServiceContext, menu *modelauthz.Menu, menuMap map[string]struct{}) {
	if len(menu.Children) > 0 {
		menu.Children = lo.Filter[*modelauthz.Menu](menu.Children, func(item *modelauthz.Menu, _ int) bool {
			var exists, matched, ok bool
			_, exists = menuMap[item.ID]
			if exists {
				if matched, _ = regexp.MatchString(item.DomainPattern, ctx.Host()); matched {
					ok = true
				}
			}
			return ok
			// if _, ok := menuMap[item.ID]; ok {
			// 	return true
			// } else {
			// 	return false
			// }
		})
		for i := range menu.Children {
			filter(ctx, menu.Children[i], menuMap)
		}
	}
}
