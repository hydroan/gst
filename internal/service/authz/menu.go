package serviceauthz

import (
	"net/http"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/samber/lo"
)

type MenuService struct {
	service.Base[*modelauthz.Menu, *modelauthz.Menu, *modelauthz.Menu]
}

// Filter pushes menu visibility down into the query condition. List and Count
// share the options this returns, so the total matches the rows the caller
// receives; filtering the slice in ListAfter instead would leave the total at
// the unfiltered count and break any client that pages on it.
//
// A subject with no visible menu yields an empty ID set, and IN over an empty
// list matches nothing, so rows and total go to zero together.
func (m *MenuService) Filter(ctx *types.ServiceContext, menu *modelauthz.Menu, opts types.QueryOptions) (*modelauthz.Menu, types.QueryOptions, error) {
	menuIDs, restricted, err := visibleMenuIDs(ctx, m.WithContext(ctx, ctx.Phase()))
	if err != nil {
		return menu, opts, err
	}
	if restricted {
		opts.Filters = append(opts.Filters, types.FilterIn(modelauthz.KeyID, menuIDs))
	}

	return menu, opts, nil
}

// ListAfter narrows expanded children, the one place role visibility cannot
// become a query condition: children are preloaded separately, so the condition
// Filter pushed down never reaches them. The top level is already handled by
// Filter and is not revisited here, which keeps the rows returned and the total
// in agreement.
func (m *MenuService) ListAfter(ctx *types.ServiceContext, data *[]*modelauthz.Menu) error {
	// Without expanded children there is nothing left to narrow, so the common
	// case resolves roles once, in Filter.
	if !lo.SomeBy(*data, func(item *modelauthz.Menu) bool { return len(item.Children) > 0 }) {
		return nil
	}
	menuIDs, restricted, err := visibleMenuIDs(ctx, m.WithContext(ctx, ctx.Phase()))
	if err != nil {
		return err
	}
	if !restricted {
		return nil
	}
	visible := make(map[string]struct{}, len(menuIDs))
	for _, id := range menuIDs {
		visible[id] = struct{}{}
	}
	for i := range *data {
		filterChildren((*data)[i], visible)
	}

	return nil
}

// visibleMenuIDs resolves the menu IDs the current subject may see. The flow
// deliberately mirrors the RBAC data model:
//   - system_root is a system-level role and bypasses menu filtering completely.
//   - RoleBinding maps the current subject ID to role IDs inside the request tenant.
//   - when the subject has no RoleBinding records, default roles in the request
//     tenant provide the fallback menu set.
//   - Role.MenuIDs is the only source of visible menus. The same set grants the
//     backend route permissions; this service is only shaping the frontend menu
//     tree.
//
// restricted reports whether the caller has to narrow by the returned set. It is
// false only for system_root. A subject whose roles select no menu at all yields
// an empty set with restricted true, which the caller turns into an empty result.
func visibleMenuIDs(ctx *types.ServiceContext, log types.Logger) ([]string, bool, error) {
	systemRoot, err := rbac.RBAC().HasSystemRole(ctx, ctx.UserID(), consts.AUTHZ_SYSTEM_ROLE_ROOT)
	if err != nil {
		return nil, false, service.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
	}
	if systemRoot {
		return nil, false, nil
	}
	// No tenant is named in the queries below: the rows they reach are already
	// scoped to the tenant this request acts in.
	roleBindings := make([]*modelauthz.RoleBinding, 0)
	if err := database.Database[*modelauthz.RoleBinding](ctx).
		WithQuery(&modelauthz.RoleBinding{SubjectID: ctx.UserID()}).
		List(&roleBindings); err != nil {
		return nil, false, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load role bindings", err)
	}

	roles := make([]*modelauthz.Role, 0)
	if len(roleBindings) > 0 {
		roleIDs := make([]string, 0, len(roleBindings))
		for _, binding := range roleBindings {
			if len(binding.RoleID) > 0 {
				roleIDs = append(roleIDs, binding.RoleID)
			}
		}
		if len(roleIDs) == 0 {
			log.Warn("subject has role-binding records but no valid role ids")
			return nil, true, nil
		}
		if err := database.Database[*modelauthz.Role](ctx).
			WithQuery(nil, types.QueryOptions{Filters: []types.Filter{types.FilterIn(modelauthz.KeyID, roleIDs)}}).
			List(&roles); err != nil {
			return nil, false, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load roles", err)
		}
		if len(roles) == 0 {
			log.Warn("subject has role-binding records but no matching roles")
			return nil, true, nil
		}
	} else {
		if err := database.Database[*modelauthz.Role](ctx).
			WithQuery(&modelauthz.Role{Default: new(true)}).
			List(&roles); err != nil {
			return nil, false, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load roles", err)
		}
	}
	if len(roles) == 0 {
		log.Warn("user has no roles and don't have default role")
		return nil, true, nil
	}
	for _, r := range roles {
		log.Infow("role", "username", ctx.Username(), "role_name", r.Name)
	}

	seen := make(map[string]struct{})
	menuIDs := make([]string, 0)
	for _, role := range roles {
		for _, id := range role.MenuIDs {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			menuIDs = append(menuIDs, id)
		}
	}

	return menuIDs, true, nil
}

// filterChildren narrows preloaded children by the same rule as the top level,
// dropping every subtree the visible set does not name. Children are loaded by a
// separate preload, so the condition Filter pushed down never applies to them.
// Callers that are not narrowing at all skip this entirely.
func filterChildren(menu *modelauthz.Menu, visible map[string]struct{}) {
	if len(menu.Children) == 0 {
		return
	}
	menu.Children = lo.Filter(menu.Children, func(item *modelauthz.Menu, _ int) bool {
		_, ok := visible[item.ID]
		return ok
	})
	for i := range menu.Children {
		filterChildren(menu.Children[i], visible)
	}
}
