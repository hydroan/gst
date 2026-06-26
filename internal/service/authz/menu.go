package serviceauthz

import (
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

// filterByRole reduces the menu tree to the menus visible to the current user.
//
// The flow deliberately mirrors the RBAC data model:
//   - root is a built-in user ID and bypasses menu filtering completely.
//   - UserRole maps the current user ID to role IDs.
//   - when the user has no UserRole records, default roles provide the fallback menu set.
//   - Role.MenuIDs grants fully selected menus; Role.MenuPartialIDs keeps parent
//     menu nodes visible when only part of their children are selected.
//   - Menu.DomainPattern still constrains visibility by the current request host.
func (m *MenuService) filterByRole(ctx *types.ServiceContext, data *[]*modelauthz.Menu, log types.Logger) error {
	// The built-in root account is identified by the stable user ID.
	if ctx.UserID() == consts.AUTHZ_USER_ROOT {
		return nil
	}

	var (
		userRoles = make([]*modelauthz.UserRole, 0)
		roles     = make([]*modelauthz.Role, 0)
	)

	if err := database.Database[*modelauthz.UserRole](ctx).
		WithQuery(&modelauthz.UserRole{UserID: ctx.UserID()}).
		List(&userRoles); err != nil {
		log.Error(err)
		return err
	}

	if len(userRoles) > 0 {
		roleIDs := make([]string, 0)
		for _, ur := range userRoles {
			if len(ur.RoleID) > 0 {
				roleIDs = append(roleIDs, ur.RoleID)
			}
		}
		if len(roleIDs) == 0 {
			log.Warn("user has user-role records but no valid role ids")
			*data = make([]*modelauthz.Menu, 0)
			return nil
		}
		if err := database.Database[*modelauthz.Role](ctx).
			WithQuery(&modelauthz.Role{Base: model.Base{ID: strings.Join(roleIDs, ",")}}).List(&roles); err != nil {
			log.Error(err)
			return err
		}
		if len(roles) == 0 {
			log.Warn("user has user-role records but no matching roles")
			*data = make([]*modelauthz.Menu, 0)
			return nil
		}
	}
	if len(userRoles) == 0 {
		if err := database.Database[*modelauthz.Role](ctx).
			WithQuery(&modelauthz.Role{Default: new(true)}).
			List(&roles); err != nil {
			log.Error(err)
			return err
		}
	}
	if len(roles) == 0 {
		log.Warn("user has no roles and don't have default role")
		*data = make([]*modelauthz.Menu, 0)
		return nil
	}
	for _, r := range roles {
		log.Infow("role", "username", ctx.Username(), "role_name", r.Name, "role_code", r.Code)
	}

	// MenuIDs and MenuPartialIDs both affect menu visibility. Only MenuIDs grants
	// backend route permissions; this service is only shaping the frontend menu tree.
	menuMap := make(map[string]struct{})
	for _, role := range roles {
		for _, id := range role.MenuIDs {
			menuMap[id] = struct{}{}
		}
		for _, id := range role.MenuPartialIDs {
			menuMap[id] = struct{}{}
		}
	}

	filtered := lo.Filter[*modelauthz.Menu](*data, func(item *modelauthz.Menu, _ int) bool {
		return menuAllowed(ctx, item, menuMap)
	})
	for i := range filtered {
		filterMenuTree(ctx, filtered[i], menuMap)
	}
	*data = filtered
	return nil
}

// filterMenuTree applies the same role and domain visibility rules recursively to
// children. The top-level list has already been filtered before this function runs.
func filterMenuTree(ctx *types.ServiceContext, menu *modelauthz.Menu, menuMap map[string]struct{}) {
	if len(menu.Children) > 0 {
		menu.Children = lo.Filter[*modelauthz.Menu](menu.Children, func(item *modelauthz.Menu, _ int) bool {
			return menuAllowed(ctx, item, menuMap)
		})
		for i := range menu.Children {
			filterMenuTree(ctx, menu.Children[i], menuMap)
		}
	}
}

// menuAllowed requires both a role/menu match and a host match. This keeps menu
// visibility aligned with role assignment while still allowing one menu table to
// serve different domains.
func menuAllowed(ctx *types.ServiceContext, menu *modelauthz.Menu, menuMap map[string]struct{}) bool {
	if _, exists := menuMap[menu.ID]; !exists {
		return false
	}
	matched, _ := regexp.MatchString(menu.DomainPattern, ctx.Host())
	return matched
}
