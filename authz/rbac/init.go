package rbac

import (
	"context"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types/consts"
)

// policyTable is the table the Casbin adapter reads and writes. Its schema is
// owned by the CasbinRule model, so the name has to agree with that model's
// GetTableName.
const policyTable = "casbin_rule"

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
# 3) the policy is written for the implicit "authenticated" role. This branch
#    checks no role membership and no tenant, so it reaches every authenticated
#    subject in every tenant — including subjects that hold no role at all. It
#    still requires a subject: authorization runs after authentication, and the
#    emptiness check keeps that a property of the matcher rather than a promise
#    the caller has to keep. Reserve it for objects whose result is already
#    scoped to the caller.
# 4) the subject belongs to the policy role in the same tenant, and the object
#    and action match the stored permission.
#
# The subject/role inequality checks keep a subject named like a role from
# receiving that role through Casbin's self-match behavior.
#
# A trailing backslash continues the line, so each branch above maps to the line
# below it and the whole matcher stays readable as it grows. A comment between
# two continued lines would end the value early, so keep the four lines adjacent.
m = (r.sub != "system_root" && g2(r.sub, "system_root")) \
 || (r.sub != "admin" && g(r.sub, "admin", r.tenant)) \
 || (r.sub != "" && p.role == "authenticated" && keyMatch3(r.obj, p.obj) && r.act == p.act) \
 || (r.sub != p.role && r.tenant == p.tenant && g(r.sub, p.role, r.tenant) && keyMatch3(r.obj, p.obj) && r.act == p.act)
`)

// Init initializes the tenant-aware Casbin enforcer when RBAC is enabled.
func Init() (err error) {
	if !config.App.Auth.RBACEnabled {
		return nil
	}

	// The adapter is told not to migrate: the policy table belongs to the
	// registered CasbinRule model, so it is created and indexed by the same
	// migration path as every other table. Letting the adapter migrate too would
	// mean two definitions of one table, and would issue DDL at startup even
	// where the framework deliberately leaves schema changes to gg migrate.
	//
	policyAdapter := newAdapter(database.DB(), policyTable)
	model, err := casbinmodel.NewModelFromString(string(modelData))
	if err != nil {
		return errors.Wrap(err, "failed to create casbin model")
	}
	contextEnforcer, err := casbin.NewContextEnforcer(model, policyAdapter)
	if err != nil {
		return errors.Wrap(err, "failed to create casbin enforcer")
	}
	var ok bool
	enforcer, ok = contextEnforcer.(*casbin.ContextEnforcer)
	if !ok {
		return errors.New("failed to create context casbin enforcer")
	}

	enforcer.SetLogger(logger.Casbin)
	enforcer.EnableEnforce(true)
	// Writes go through mutate, which drives the adapter itself so it can split
	// the database half from the in-memory half. Casbin's own persistence would
	// do both at once, which is what leaves memory ahead of a rolled back
	// transaction.
	enforcer.EnableAutoSave(false)
	policyStore = policyAdapter

	for _, subject := range defaultSystemRootSubjects {
		if err := RBAC().AssignSystemRole(context.Background(), subject, defaultSystemRole); err != nil {
			return errors.Wrapf(err, "failed to add default system role for %s", subject)
		}
	}
	return nil
}
