package rbac

import (
	"context"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types/consts"
)

var adapter *gormadapter.Adapter

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
# 2) the subject belongs to the built-in admin role in the request tenant.
#    This branch grants unconditional access to every object/action in the
#    tenant — it does NOT check any p (permission policy) entry, unlike
#    branch 3 below. Assigning the "admin" role is equivalent to granting
#    full tenant-scoped superuser access.
# 3) the subject belongs to the policy role in the same tenant, and the object
#    and action match the stored permission.
#
# The subject/role inequality checks keep a subject named like a role from
# receiving that role through Casbin's self-match behavior.
m = (r.sub != "system_root" && g2(r.sub, "system_root")) || (r.sub != "admin" && g(r.sub, "admin", r.tenant)) || (r.sub != p.role && r.tenant == p.tenant && g(r.sub, p.role, r.tenant) && keyMatch3(r.obj, p.obj) && r.act == p.act)
`)

// Init initializes the tenant-aware Casbin enforcer when RBAC is enabled.
func Init() (err error) {
	if !config.App.Auth.RBACEnabled {
		return nil
	}

	// gormadapter.NewAdapterByDBWithCustomTable creates the Casbin policy table
	// with an auto-incrementing primary key managed by the adapter.
	if adapter, err = gormadapter.NewAdapterByDBWithCustomTable(database.DB(), new(casbinRule), "casbin_rule"); err != nil {
		return errors.Wrap(err, "failed to create casbin adapter")
	}
	model, err := casbinmodel.NewModelFromString(string(modelData))
	if err != nil {
		return errors.Wrap(err, "failed to create casbin model")
	}
	contextEnforcer, err := casbin.NewContextEnforcer(model, adapter)
	if err != nil {
		return errors.Wrap(err, "failed to create casbin enforcer")
	}
	var ok bool
	enforcer, ok = contextEnforcer.(*casbin.ContextEnforcer)
	if !ok {
		return errors.New("failed to create context casbin enforcer")
	}

	enforcer.SetLogger(logger.Casbin)
	enforcer.EnableAutoSave(true)
	enforcer.EnableEnforce(true)

	for _, subject := range defaultSystemRootSubjects {
		if err := RBAC().AssignSystemRole(context.Background(), subject, defaultSystemRole); err != nil {
			return errors.Wrapf(err, "failed to add default system role for %s", subject)
		}
	}
	return nil
}
