package rbac

import (
	"context"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
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
# The matcher below never names g2. It is declared so Casbin builds a role
# manager for it, which is what authorize asks about system-level membership;
# removing it would leave that question with nothing to answer it.
g2 = _, _

[policy_effect]
# Allow the request if any matched policy effect is "allow".
e = some(where (p.eft == allow))

[matchers]
# Every branch here consults a policy. The two that do not — system_root, and
# the built-in admin role inside the request tenant — are decided by authorize
# before the engine is entered, because Casbin evaluates this expression once
# per stored policy row and a branch ignoring p would be recomputed for each of
# them. Keep it that way: a branch added here that does not read p is paid for
# by every policy in the deployment, on every request.
#
# Allow a request when either:
# 1) the policy is written for the implicit "authenticated" role. This branch
#    checks no role membership and no tenant, so it reaches every authenticated
#    subject in every tenant — including subjects that hold no role at all. It
#    still requires a subject: authorization runs after authentication, and the
#    emptiness check keeps that a property of the matcher rather than a promise
#    the caller has to keep. Reserve it for objects whose result is already
#    scoped to the caller.
# 2) the subject belongs to the policy role in the same tenant, and the object
#    and action match the stored permission.
#
# The subject/role inequality check keeps a subject named like a role from
# receiving that role through Casbin's self-match behavior.
#
# A trailing backslash continues the line, so each branch above maps to the line
# below it and the whole matcher stays readable as it grows. A comment between
# two continued lines would end the value early, so keep the two lines adjacent.
m = (r.sub != "" && p.role == "authenticated" && pathMatch(r.obj, p.obj) && r.act == p.act) \
 || (r.sub != p.role && r.tenant == p.tenant && g(r.sub, p.role, r.tenant) && pathMatch(r.obj, p.obj) && r.act == p.act)
`)

// newEnforcer builds the enforcer modelData describes, with the invariants the
// package depends on already in place.
//
// Construction, autosave and the matcher functions are one step on purpose. The
// enforcer compiles its matcher once and caches it together with the function
// map it was built from, so a function registered after the first Enforce is
// never seen by the cached expression, and the symptom is a matcher that cannot
// resolve pathMatch at all rather than a slow one. Leaving that ordering to
// each construction site to remember is the kind of convention the second site
// gets wrong.
func newEnforcer(store persist.ContextAdapter) (*casbin.ContextEnforcer, error) {
	model, err := casbinmodel.NewModelFromString(string(modelData))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create casbin model")
	}
	contextEnforcer, err := casbin.NewContextEnforcer(model, store)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create casbin enforcer")
	}
	enforcer, ok := contextEnforcer.(*casbin.ContextEnforcer)
	if !ok {
		return nil, errors.New("failed to create context casbin enforcer")
	}

	enforcer.AddFunction(matcherFuncPathMatch, pathMatchFunc)
	// Writes go through mutate, which drives the adapter itself so it can split
	// the database half from the in-memory half. Casbin's own persistence would
	// do both at once, which is what leaves memory ahead of a rolled back
	// transaction.
	enforcer.EnableAutoSave(false)

	return enforcer, nil
}

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
	policyAdapter := newAdapter(database.DB(), policyTable)
	if enforcer, err = newEnforcer(policyAdapter); err != nil {
		return err
	}

	// No logger is given to the enforcer, and none should be. Of the events
	// Casbin reports, the only one it raises along any path this package takes
	// is the enforcement event, once per decision and so once per request —
	// carrying neither the tenant, nor what allowed the request, nor the trace
	// it belongs to, all of which the authz middleware already writes for the
	// same decision. The rest are raised from entry points this package never
	// calls: policy writes go through mutate rather than the enforcer, and the
	// reload goes through LoadPolicyCtx, which reports nothing at all.
	enforcer.EnableEnforce(true)
	policyStore = policyAdapter

	for _, subject := range defaultSystemRootSubjects {
		if err := RBAC().AssignSystemRole(context.Background(), subject, defaultSystemRole); err != nil {
			return errors.Wrapf(err, "failed to add default system role for %s", subject)
		}
	}
	return nil
}
