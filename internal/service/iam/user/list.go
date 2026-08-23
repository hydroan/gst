package serviceiamuser

import (
	"net/http"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/internal/service/iam/adminauth"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// AdminUserListService handles GET /iam/admin/users for privileged administrators.
//
// Authorization is split into two steps: List first checks whether the actor may
// call the admin users endpoint, then listUsers builds the tenant-visible user
// visibility scope used by the database query.
type AdminUserListService struct {
	service.Base[*modeliamuser.User, *model.Empty, *modeliamuser.AdminUserListRsp]
}

// List returns users visible to the current administrator.
//
// Passing nil as the target to EnsureTenantAdmin means this call only verifies
// endpoint-level permission. The visible user set is applied later by
// userVisibilityQueryOptions because list requests do not have one concrete target
// user to check.
func (a *AdminUserListService) List(ctx *types.ServiceContext, _ *model.Empty) (rsp *modeliamuser.AdminUserListRsp, err error) {
	actor, err := LoadActor(ctx)
	if err != nil {
		return nil, err
	}
	if err = adminauth.EnsureTenantAdmin(ctx, actor, nil); err != nil {
		return nil, err
	}

	users, total, err := a.listUsers(ctx, actor)
	if err != nil {
		return nil, err
	}
	views, err := buildAdminUserViews(ctx, users)
	if err != nil {
		return nil, err
	}

	return &modeliamuser.AdminUserListRsp{
		Items: views,
		Total: total,
	}, nil
}

// listUsers applies the authorization-derived scope and the request's own
// query parameters, counts the full filtered set, then pages the returned slice.
//
// The parameters are parsed by the service base, which is the same parsing the
// framework list controller does; this action only reaches for it directly
// because declaring a Result type takes the request over from that controller.
// Doing it by hand here instead is what previously left this endpoint with a
// username filter and paging while every other list also answered to ordering
// and operator filters.
//
// The count and the page are built from one query value and one set of options,
// because a total computed from anything else describes a different result set
// than the page beside it.
func (a *AdminUserListService) listUsers(ctx *types.ServiceContext, actor *modeliamuser.User) ([]*modeliamuser.User, int, error) {
	opts, err := userVisibilityQueryOptions(ctx, actor)
	if err != nil {
		return nil, 0, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list users", err)
	}

	userQuery, err := a.QueryModel(ctx)
	if err != nil {
		return nil, 0, service.NewError(http.StatusBadRequest, err.Error())
	}
	filters, err := a.QueryFilters(ctx)
	if err != nil {
		return nil, 0, service.NewError(http.StatusBadRequest, err.Error())
	}
	orders, err := a.QueryOrders(ctx)
	if err != nil {
		return nil, 0, service.NewError(http.StatusBadRequest, err.Error())
	}
	// The visibility filters come first: they are the scope the client cannot
	// influence, and appending the client's own filters to them can only narrow
	// what is already allowed.
	opts.Filters = append(opts.Filters, filters...)
	opts.PresentFields = a.QueryPresentFields(ctx)

	var total int
	if err = database.Database[*modeliamuser.User](ctx).WithQuery(userQuery, opts).Count(&total); err != nil {
		return nil, 0, service.NewErrorWithCause(http.StatusInternalServerError, "failed to count users", err)
	}

	if len(orders) == 0 {
		orders = []types.Order{types.Desc("created_at")}
	}
	page, size := a.QueryPagination(ctx)
	users := make([]*modeliamuser.User, 0)
	if err = database.Database[*modeliamuser.User](ctx).
		WithQuery(userQuery, opts).
		WithOrder(orders...).
		WithPagination(page, size).
		List(&users); err != nil {
		return nil, 0, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list users", err)
	}
	return users, total, nil
}

// userVisibilityQueryOptions converts IAM admin visibility rules into a database query.
//
// IAM users do not store tenant_id. Tenant membership comes from RBAC role
// bindings, so tenant administrators are scoped by first reading all subjects
// assigned to at least one role in the current tenant, then querying users by
// those subject IDs. System root actors bypass this tenant scope and can list
// every user.
func userVisibilityQueryOptions(ctx *types.ServiceContext, actor *modeliamuser.User) (types.QueryOptions, error) {
	systemRoot, err := isSystemRoot(ctx, actor)
	if err != nil {
		return types.QueryOptions{}, service.NewErrorWithCause(http.StatusInternalServerError, "failed to resolve actor system role", err)
	}
	if systemRoot {
		return types.QueryOptions{AllowEmpty: true}, nil
	}

	// The current tenant comes from the request context and falls back to the
	// default authorization domain when the application has no tenant resolver.
	subjectIDs, err := rbac.RBAC().SubjectsInTenant(ctx, currentTenant(ctx))
	if err != nil {
		return types.QueryOptions{}, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list tenant subjects", err)
	}
	if len(subjectIDs) == 0 {
		return emptyUserVisibilityQueryOptions(), nil
	}
	subjectIDs, err = excludeSystemRootSubjects(ctx, subjectIDs)
	if err != nil {
		return types.QueryOptions{}, err
	}
	if len(subjectIDs) == 0 {
		return emptyUserVisibilityQueryOptions(), nil
	}
	return types.QueryOptions{Filters: []types.Filter{types.FilterIn("id", subjectIDs)}}, nil
}

func emptyUserVisibilityQueryOptions() types.QueryOptions {
	return types.QueryOptions{RawQuery: "1 = 0", AllowEmpty: true}
}

// excludeSystemRootSubjects removes subjects that tenant administrators must
// never manage through tenant-local user APIs. A root user can be bound to a
// tenant role for authorization setup, but that binding must not make root
// visible or manageable from that tenant's admin user list.
func excludeSystemRootSubjects(ctx *types.ServiceContext, subjectIDs []string) ([]string, error) {
	filtered := make([]string, 0, len(subjectIDs))
	for _, subjectID := range subjectIDs {
		systemRoot, err := rbac.RBAC().HasSystemRole(ctx, subjectID, consts.AUTHZ_SYSTEM_ROLE_ROOT)
		if err != nil {
			return nil, service.NewErrorWithCause(http.StatusInternalServerError, "authorization unavailable", err)
		}
		if systemRoot {
			continue
		}
		filtered = append(filtered, subjectID)
	}
	return filtered, nil
}
