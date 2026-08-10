package serviceiamuser

import (
	"net/http"
	"strconv"

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

type adminUserListFilters struct {
	Username string
	Page     int
	Size     int
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

	users, total, err := listUsers(ctx, actor)
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

// listUsers applies the authorization-derived scope and request filters, counts
// the full filtered result set, then applies request pagination only to the
// returned page.
func listUsers(ctx *types.ServiceContext, actor *modeliamuser.User) ([]*modeliamuser.User, int, error) {
	opts, err := userVisibilityQueryOptions(ctx, actor)
	if err != nil {
		return nil, 0, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list users", err)
	}
	filters := readAdminUserListFilters(ctx)
	userQuery, opts := adminUserListQuery(filters, opts)

	var total int
	if err = database.Database[*modeliamuser.User](ctx).WithQuery(userQuery, opts).Count(&total); err != nil {
		return nil, 0, service.NewErrorWithCause(http.StatusInternalServerError, "failed to count users", err)
	}

	// WithPagination(0, 0) falls back to page 1 with the default limit instead
	// of disabling pagination, so an unpaginated request needs its own chain.
	users := make([]*modeliamuser.User, 0)
	if filters.Page > 0 || filters.Size > 0 {
		err = database.Database[*modeliamuser.User](ctx).
			WithQuery(userQuery, opts).
			WithOrder(types.Desc("created_at")).
			WithPagination(filters.Page, filters.Size).
			List(&users)
	} else {
		err = database.Database[*modeliamuser.User](ctx).
			WithQuery(userQuery, opts).
			WithOrder(types.Desc("created_at")).
			List(&users)
	}
	if err != nil {
		return nil, 0, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list users", err)
	}
	return users, total, nil
}

// readAdminUserListFilters reads the URL query parameters supported by GET
// /iam/admin/users. The endpoint has no request body; pagination and username
// filters are carried by the URL query string.
func readAdminUserListFilters(ctx *types.ServiceContext) adminUserListFilters {
	query := ctx.Query()
	return adminUserListFilters{
		Username: query.Get("username"),
		Page:     parseAdminUserListInt(query.Get(consts.QUERY_PAGE)),
		Size:     parseAdminUserListInt(query.Get(consts.QUERY_SIZE)),
	}
}

func parseAdminUserListInt(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

// adminUserListQuery converts URL filters into the model query consumed by
// database.WithQuery. Tenant visibility remains in opts (a Filters IN condition, or the
// fail-closed RawQuery for an empty visible set) and WithQuery combines it
// with the username condition using AND semantics. The username searches as a
// substring through the like operator filter, the framework's one fuzzy path.
func adminUserListQuery(filters adminUserListFilters, opts types.QueryOptions) (*modeliamuser.User, types.QueryOptions) {
	query := new(modeliamuser.User)
	if filters.Username == "" {
		return query, opts
	}
	opts.Filters = append(opts.Filters, types.FilterLike("username", filters.Username))
	return query, opts
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
