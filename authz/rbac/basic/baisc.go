package basic

import (
	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types/consts"
)

var defaultAdmins = []string{
	consts.AUTHZ_USER_ROOT,
}

var defaultAdminRole = consts.AUTHZ_ROLE_ADMIN

// addGroupingPolicy adds a user-role relationship while skipping self-referential entries.
// Casbin treats identical subject and role names as a link, but gst keeps users and
// roles distinct so role names cannot grant permissions to same-named subjects.
func addGroupingPolicy(enforcer *casbin.SyncedEnforcer, subject string, role string) error {
	if subject == role {
		return nil
	}
	_, err := enforcer.AddGroupingPolicy(subject, role)
	return err
}

var modelData = []byte(`
[request_definition]
# r defines the incoming request tuple:
# sub: subject (user or role identifier)
# obj: object (requested resource path, e.g., /api/users/123)
# act: action (HTTP method, e.g., GET/POST/PUT/DELETE/PATCH)
r = sub, obj, act

[policy_definition]
# p defines the stored policy tuple:
# sub: policy subject (typically a role name, e.g., "editor")
# obj: policy object (resource template, e.g., /api/users/{id})
# act: policy action (HTTP method)
# eft: effect ("allow")
p = sub, obj, act, eft

[role_definition]
# g defines role membership:
# g(user, role) means "user" belongs to "role"
g = _, _

[policy_effect]
# Effect aggregator: allow if any matched policy’s eft is "allow".
e = some(where (p.eft == allow))

[matchers]
# Matcher logic:
# 1) Admin bypass: if subject belongs to "admin", allow
# 2) Otherwise: require role match AND path match AND method match
# 3) Subjects must not equal role names, preventing Casbin's self-match behavior
#    from turning a username or user id like "admin" into the admin role.
#    - g(r.sub, p.sub): subject belongs to policy role
#    - keyMatch3(r.obj, p.obj): REST path template matches (e.g., /api/users/{id})
#    - r.act == p.act: HTTP method equals
m = (r.sub != "admin" && g(r.sub, "admin")) || (r.sub != p.sub && g(r.sub, p.sub) && keyMatch3(r.obj, p.obj) && r.act == p.act)
`)

func Init() (err error) {
	if !config.App.Auth.RBACEnable {
		return nil
	}

	// NOTE: gormadapter.NewAdapterByDBWithCustomTable creates the Casbin policy table with an auto-incrementing primary key.
	if rbac.Adapter, err = gormadapter.NewAdapterByDBWithCustomTable(database.DB(), new(modelauthz.CasbinRule)); err != nil {
		return errors.Wrap(err, "failed to create casbin adapter")
	}
	model, err := casbinmodel.NewModelFromString(string(modelData))
	if err != nil {
		return errors.Wrap(err, "failed to create casbin model")
	}
	if rbac.Enforcer, err = casbin.NewSyncedEnforcer(model, rbac.Adapter); err != nil {
		return errors.Wrap(err, "failed to create casbin enforcer")
	}

	rbac.Enforcer.SetLogger(logger.Casbin)
	rbac.Enforcer.EnableAutoSave(true)
	rbac.Enforcer.EnableEnforce(true)

	for _, user := range defaultAdmins {
		if err := addGroupingPolicy(rbac.Enforcer, user, defaultAdminRole); err != nil {
			return errors.Wrapf(err, "failed to add default admin grouping policy for %s", user)
		}
	}
	if err := addGroupingPolicy(rbac.Enforcer, consts.AUTHZ_USER_BLOCKED, consts.AUTHZ_ROLE_BLOCKED); err != nil {
		return errors.Wrap(err, "failed to add blocked grouping policy")
	}

	return nil
}
