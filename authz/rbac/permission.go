package rbac

import (
	"context"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// RemoveRole removes all policies and subject assignments for role in tenant.
func (r *rbac) RemoveRole(ctx context.Context, tenant string, role string) (err error) {
	tenant = normalizeTenant(tenant)
	ctx, finishSpan := traceRBAC(ctx, "remove_role", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(
		ctx,
		removeFiltered("p", "p", 0, tenant, role),
		removeFiltered("g", "g", 1, role, tenant),
	)
}

// ErrReservedRole reports a permission written against a role the matcher reads
// as something other than an ordinary role.
//
// A policy stored for the implicit authenticated role is matched without a role
// membership check and without a tenant check, so it allows every subject that
// can log in, in every tenant. Written through an ordinary role call it would
// turn one tenant's permission set into a grant across the whole deployment,
// while reading in the records it came from as a role like any other.
//
// SetPermissionsForAuthenticated writes that rule deliberately, and refusing it
// here is what leaves that method as the only way to write it. Removals are not
// refused: whatever is already stored has to be removable.
var ErrReservedRole = errors.New("rbac: role is reserved by the authorization matcher")

// errIfReservedRole refuses a permission written against a reserved role. The
// comparison is exact, because the matcher's is: a role differing by so much as
// a space is an ordinary role that reaches nothing on its own.
func errIfReservedRole(role string) error {
	if role == consts.AUTHZ_ROLE_AUTHENTICATED {
		return errors.Wrapf(ErrReservedRole, "role %q", role)
	}
	return nil
}

// authenticatedPolicyTenant is the tenant column stored for policies written
// against consts.AUTHZ_ROLE_AUTHENTICATED. The matcher branch for that role
// compares no tenant, so the value never takes part in a decision; it marks the
// row as tenant-independent for anyone reading casbin_rule.
const authenticatedPolicyTenant = "*"

// SetRolePermissions replaces the entire permission set held by role in tenant.
func (r *rbac) SetRolePermissions(
	ctx context.Context, tenant string, role string, permissions []types.Permission,
) (err error) {
	if err = errIfReservedRole(role); err != nil {
		return err
	}
	tenant = normalizeTenant(tenant)
	ctx, finishSpan := traceRBAC(ctx, "set_role_permissions", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.replacePermissions(ctx, tenant, role, permissions)
}

// SetPermissionsForAuthenticated replaces every permission held by all
// authenticated subjects with permissions. The policies are stored against the
// implicit authenticated role, which no grouping rule ever assigns, so they
// reach subjects holding no role at all.
//
// It shares its replacement with SetRolePermissions: the implicit role differs
// from a real one only in the tenant and role the policies are stored under, and
// keeping one implementation means the atomicity both depend on cannot hold for
// one of them and not the other.
func (r *rbac) SetPermissionsForAuthenticated(ctx context.Context, permissions []types.Permission) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "set_permissions_for_authenticated",
		rbacTraceFields(authenticatedPolicyTenant, consts.AUTHZ_ROLE_AUTHENTICATED))
	defer func() {
		finishSpan(err)
	}()

	return r.replacePermissions(ctx, authenticatedPolicyTenant, consts.AUTHZ_ROLE_AUTHENTICATED, permissions)
}

// replacePermissions swaps the whole permission set stored for role in tenant
// under a single write lock, so a concurrent Authorize sees either the old set
// or the new one and never the gap between them.
//
// The two engine calls form an ordering the correctness of the batch depends on:
// Casbin skips a batch insert entirely, and reports no error, when any rule in
// it already exists. Clearing the set first is what keeps that from silently
// dropping the whole replacement, so the delete must stay ahead of the insert.
func (r *rbac) replacePermissions(
	ctx context.Context, tenant string, role string, permissions []types.Permission,
) error {
	mutations := []policyMutation{removeFiltered("p", "p", 0, tenant, role)}
	if rules := permissionPolicies(tenant, role, permissions); len(rules) > 0 {
		mutations = append(mutations, addRules("p", "p", rules...))
	}
	return r.mutate(ctx, mutations...)
}

// permissionPolicies renders permissions as p policy rows for role in tenant.
//
// It drops repeats because Casbin's batch insert does not deduplicate within a
// batch, so a caller listing the same permission twice would otherwise store it
// twice. It sorts because the stored order decides which rule Casbin reports as
// the matched one: an order that followed the caller's would make
// a decision name a different rule for the same request whenever the
// caller derived its set from an unordered source.
//
// An entry missing an object or an action is rendered as it stands rather than
// dropped, and mutate refuses the rule before anything is written. Dropping it
// would leave a caller whose entries are all empty having asked to revoke
// everything, silently: the replacement deletes the role's current set whether
// or not a new one follows. Leaving the refusal to mutate is also what keeps
// one implementation of it — the same ErrEmptyPolicyValue every other rule kind
// is checked against.
func permissionPolicies(tenant string, role string, permissions []types.Permission) [][]string {
	unique := make([]types.Permission, 0, len(permissions))
	seen := make(map[types.Permission]struct{}, len(permissions))
	for _, permission := range permissions {
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		unique = append(unique, permission)
	}
	slices.SortFunc(unique, func(a, b types.Permission) int {
		if c := strings.Compare(a.Object, b.Object); c != 0 {
			return c
		}
		return strings.Compare(a.Action, b.Action)
	})

	rules := make([][]string, 0, len(unique))
	for _, permission := range unique {
		rules = append(rules, []string{
			tenant, role, permission.Object, permission.Action, string(consts.EffectAllow),
		})
	}
	return rules
}
