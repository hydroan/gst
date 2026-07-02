package serviceiamuser

import (
	"strconv"

	"github.com/cockroachdb/errors"
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
	Page     uint
	Size     uint
}

// List returns users visible to the current administrator.
//
// Passing nil as the target to EnsureTenantAdmin means this call only verifies
// endpoint-level permission. The visible user set is applied later by
// userVisibilityQueryConfig because list requests do not have one concrete target
// user to check.
func (a *AdminUserListService) List(ctx *types.ServiceContext, _ *model.Empty) (rsp *modeliamuser.AdminUserListRsp, err error) {
	log := a.WithContext(ctx, ctx.Phase())

	actor, err := LoadActor(ctx)
	if err != nil {
		log.Error("failed to resolve actor user", err)
		return nil, err
	}
	if err = adminauth.EnsureTenantAdmin(ctx, actor, nil); err != nil {
		log.Error("admin user list denied", err)
		return nil, err
	}

	users, total, err := listUsers(ctx, actor)
	if err != nil {
		log.Error("failed to list admin users", err)
		return nil, err
	}
	views, err := buildAdminUserViews(ctx, users)
	if err != nil {
		log.Error("failed to build admin user views", err)
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
func listUsers(ctx *types.ServiceContext, actor *modeliamuser.User) ([]*modeliamuser.User, int64, error) {
	cfg, err := userVisibilityQueryConfig(ctx, actor)
	if err != nil {
		return nil, 0, err
	}
	filters := readAdminUserListFilters(ctx)
	userQuery, cfg := adminUserListQuery(filters, cfg)

	var total int64
	if err = database.Database[*modeliamuser.User](ctx).WithQuery(userQuery, cfg).Count(&total); err != nil {
		return nil, 0, err
	}

	users := make([]*modeliamuser.User, 0)
	query := database.Database[*modeliamuser.User](ctx).
		WithQuery(userQuery, cfg).
		WithOrder("created_at DESC")
	if filters.Page > 0 || filters.Size > 0 {
		query = query.WithPagination(int(filters.Page), int(filters.Size))
	}
	if err = query.List(&users); err != nil {
		return nil, 0, err
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
		Page:     parseAdminUserListUint(query.Get(consts.QUERY_PAGE)),
		Size:     parseAdminUserListUint(query.Get(consts.QUERY_SIZE)),
	}
}

func parseAdminUserListUint(value string) uint {
	parsed, err := strconv.ParseUint(value, 10, 0)
	if err != nil {
		return 0
	}
	return uint(parsed)
}

// adminUserListQuery converts URL filters into the model query consumed by
// database.WithQuery. Tenant visibility remains in cfg.RawQuery and WithQuery
// combines it with the username condition using AND semantics.
func adminUserListQuery(filters adminUserListFilters, cfg types.QueryConfig) (*modeliamuser.User, types.QueryConfig) {
	query := new(modeliamuser.User)
	if filters.Username == "" {
		return query, cfg
	}
	query.Username = filters.Username
	cfg.FuzzyMatch = true
	return query, cfg
}

// userVisibilityQueryConfig converts IAM admin visibility rules into a database query.
//
// IAM users do not store tenant_id. Tenant membership comes from RBAC role
// bindings, so tenant administrators are scoped by first reading all subjects
// assigned to at least one role in the current tenant, then querying users by
// those subject IDs. System root actors bypass this tenant scope and can list
// every user.
func userVisibilityQueryConfig(ctx *types.ServiceContext, actor *modeliamuser.User) (types.QueryConfig, error) {
	systemRoot, err := isSystemRoot(ctx, actor)
	if err != nil {
		return types.QueryConfig{}, errors.Wrap(err, "failed to resolve actor system role")
	}
	if systemRoot {
		return types.QueryConfig{AllowEmpty: true}, nil
	}

	// The current tenant comes from the request context and falls back to the
	// default authorization domain when the application has no tenant resolver.
	subjectIDs, err := rbac.RBAC().SubjectsInTenant(ctx, currentTenant(ctx))
	if err != nil {
		return types.QueryConfig{}, errors.Wrap(err, "failed to list tenant subjects")
	}
	if len(subjectIDs) == 0 {
		return emptyUserVisibilityQueryConfig(), nil
	}
	subjectIDs, err = excludeSystemRootSubjects(ctx, subjectIDs)
	if err != nil {
		return types.QueryConfig{}, err
	}
	if len(subjectIDs) == 0 {
		return emptyUserVisibilityQueryConfig(), nil
	}
	return types.QueryConfig{RawQuery: "id IN ?", RawQueryArgs: []any{subjectIDs}}, nil
}

func emptyUserVisibilityQueryConfig() types.QueryConfig {
	return types.QueryConfig{RawQuery: "1 = 0", AllowEmpty: true}
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
			return nil, errors.Wrap(err, "failed to resolve subject system role")
		}
		if systemRoot {
			continue
		}
		filtered = append(filtered, subjectID)
	}
	return filtered, nil
}
