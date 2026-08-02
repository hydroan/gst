package rbac

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

// tenantRoleGrouping holds assignments inside one tenant, systemRoleGrouping
// those that sit above every tenant. Both name a grouping the model declares.
const (
	tenantRoleGrouping = "g"
	systemRoleGrouping = "g2"
)

// policyTable is the table the Casbin adapter reads and writes. Its schema is
// owned by the CasbinRule model, so the name has to agree with that model's
// GetTableName.
const policyTable = "casbin_rule"
