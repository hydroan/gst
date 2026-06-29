package rbac

import (
	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types/consts"
)

type casbinRule struct {
	ID    uint64 `gorm:"primaryKey;autoIncrement:true"`
	Ptype string `gorm:"size:100"`
	V0    string `gorm:"size:100"`
	V1    string `gorm:"size:100"`
	V2    string `gorm:"size:100"`
	V3    string `gorm:"size:100"`
	V4    string `gorm:"size:100"`
	V5    string `gorm:"size:100"`
}

func (casbinRule) TableName() string { return "casbin_rule" }

var defaultSystemRootSubjects = []string{
	consts.AUTHZ_USER_ROOT,
}

var defaultSystemRole = consts.AUTHZ_SYSTEM_ROLE_ROOT

var modelData = []byte(`
[request_definition]
# r defines the incoming authorization request tuple.
# tenant: authorization domain, defaults to "default"
# sub: subject, usually the authenticated user ID
# obj: object, usually the requested API path
# act: action, usually the HTTP method
r = tenant, sub, obj, act

[policy_definition]
# p defines a permission granted to role inside tenant.
# tenant: authorization domain
# role: role identifier stored in authz role bindings
# obj: object template, for example /api/authz/roles/{id}
# act: action, usually the HTTP method
# eft: effect, currently "allow"
p = tenant, role, obj, act, eft

[role_definition]
# g defines role membership inside a tenant:
# g(subject, role, tenant) means subject has role in tenant.
g = _, _, _
# g2 defines system-level role membership:
# g2(subject, role) means subject has role outside any tenant.
g2 = _, _

[policy_effect]
# Allow the request if any matched policy effect is "allow".
e = some(where (p.eft == allow))

[matchers]
# Allow a request when either:
# 1) the subject belongs to the system_root role through g2. This branch does
#    not compare tenant, so system_root is intentionally cross-tenant.
# 2) the subject belongs to the built-in admin role in the request tenant, or
# 3) the subject belongs to the policy role in the same tenant, and the object
#    and action match the stored permission.
#
# The subject/role inequality checks keep a subject named like a role from
# receiving that role through Casbin's self-match behavior.
m = (r.sub != "system_root" && g2(r.sub, "system_root")) || (r.sub != "admin" && g(r.sub, "admin", r.tenant)) || (r.sub != p.role && r.tenant == p.tenant && g(r.sub, p.role, r.tenant) && keyMatch3(r.obj, p.obj) && r.act == p.act)
`)

// Init initializes the tenant-aware Casbin enforcer when RBAC is enabled.
func Init() (err error) {
	if !config.App.Auth.RBACEnable {
		return nil
	}

	// gormadapter.NewAdapterByDBWithCustomTable creates the Casbin policy table
	// with an auto-incrementing primary key managed by the adapter.
	if Adapter, err = gormadapter.NewAdapterByDBWithCustomTable(database.DB(), new(casbinRule), "casbin_rule"); err != nil {
		return errors.Wrap(err, "failed to create casbin adapter")
	}
	model, err := casbinmodel.NewModelFromString(string(modelData))
	if err != nil {
		return errors.Wrap(err, "failed to create casbin model")
	}
	if Enforcer, err = casbin.NewSyncedEnforcer(model, Adapter); err != nil {
		return errors.Wrap(err, "failed to create casbin enforcer")
	}

	Enforcer.SetLogger(logger.Casbin)
	Enforcer.EnableAutoSave(true)
	Enforcer.EnableEnforce(true)

	for _, subject := range defaultSystemRootSubjects {
		if err := RBAC().AssignSystemRole(subject, defaultSystemRole); err != nil {
			return errors.Wrapf(err, "failed to add default system role for %s", subject)
		}
	}
	return nil
}
