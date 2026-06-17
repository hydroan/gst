package basic

import (
	"os"
	"path/filepath"

	"github.com/casbin/casbin/v3"
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
	consts.AUTHZ_USER_ADMIN,
}

var defaultAdminRole = consts.AUTHZ_ROLE_ADMIN

// addGroupingPolicy adds a user-role relationship while skipping self-referential
// entries such as admin -> admin. Casbin v3.10 rejects these self loops as role
// hierarchy cycles, and they do not provide any additional authorization value.
func addGroupingPolicy(enforcer *casbin.Enforcer, subject string, role string) error {
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
# eft: effect ("allow" or "deny")
p = sub, obj, act, eft

[role_definition]
# g defines role membership:
# g(user, role) means "user" belongs to "role"
g = _, _

[policy_effect]
# Effect aggregator: allow if any matched policy’s eft is "allow".
# With this, explicit "deny" does not override an existing allow.
# Alternative examples (commented):
# - priority-based: e = priority(p_eft) || some(where (p_eft == allow))
# - deny-precedence: e = some(where (p_eft == deny)) == false && some(where (p_eft == allow))
e = some(where (p.eft == allow))

[matchers]
# Matcher logic:
# 1) Admin bypass: if subject belongs to "admin", allow
# 2) Otherwise: require role match AND path match AND method match
#    - g(r.sub, p.sub): subject belongs to policy role
#    - keyMatch3(r.obj, p.obj): REST path template matches (e.g., /api/users/{id})
#    - r.act == p.act: HTTP method equals
m = g(r.sub, "admin") || (g(r.sub, p.sub) && keyMatch3(r.obj, p.obj) && r.act == p.act)
`)

func Init() (err error) {
	if !config.App.Auth.RBACEnable {
		return nil
	}

	filename := filepath.Join(config.Tempdir(), "casbin_model.conf")
	if err = os.WriteFile(filename, modelData, 0o600); err != nil {
		return errors.Wrapf(err, "failed to write model file %s", filename)
	}
	// NOTE: gormadapter.NewAdapterByDBWithCustomTable creates the Casbin policy table with an auto-incrementing primary key.
	if rbac.Adapter, err = gormadapter.NewAdapterByDBWithCustomTable(database.DB, new(modelauthz.CasbinRule)); err != nil {
		return errors.Wrap(err, "failed to create casbin adapter")
	}
	if rbac.Enforcer, err = casbin.NewEnforcer(filename, rbac.Adapter); err != nil {
		return errors.Wrap(err, "failed to create casbin enforcer")
	}

	rbac.Enforcer.SetLogger(logger.Casbin)
	rbac.Enforcer.EnableAutoSave(true)
	rbac.Enforcer.EnableAutoNotifyDispatcher(true)
	rbac.Enforcer.EnableAutoNotifyWatcher(true)
	rbac.Enforcer.EnableEnforce(true)

	for _, user := range defaultAdmins {
		if err := addGroupingPolicy(rbac.Enforcer, user, defaultAdminRole); err != nil {
			return errors.Wrapf(err, "failed to add default admin grouping policy for %s", user)
		}
	}
	if err := addGroupingPolicy(rbac.Enforcer, consts.AUTHZ_USER_BLOCKED, consts.AUTHZ_ROLE_BLOCKED); err != nil {
		return errors.Wrap(err, "failed to add blocked grouping policy")
	}

	return rbac.Enforcer.LoadPolicy()
}
