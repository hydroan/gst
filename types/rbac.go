package types

import (
	"context"

	"github.com/hydroan/gst/types/consts"
)

// Permission is one operation a role is allowed to perform on one object. It is
// the unit the whole-set replacement methods on RBAC take, so a caller states a
// role's permissions as a set rather than as a sequence of grants.
type Permission struct {
	// Object is the protected resource, usually a route path template such as
	// /api/things/{id}.
	Object string

	// Action is the operation on that object, usually an HTTP method.
	Action string
}

// Decision is the outcome of one authorization check.
//
// Source names the strongest rule that allowed the request and is empty on a
// denial, because a denial has no granting rule. MatchedRule is the policy row
// that allowed it, and is nil unless Source names a policy: the rules that
// allow without consulting one leave the engine free to report an unrelated
// row, which would read as the reason for access while being nothing of the
// kind.
//
// Reason is the mirror of Source and is set only on a denial, where naming a
// rule is not possible and what an operator needs instead is which step is
// missing. It is empty when the implementation could not tell, so an empty
// reason on a denial means unknown rather than none.
type Decision struct {
	Allowed     bool
	Source      consts.GrantSource
	Reason      consts.DenyReason
	MatchedRule []string
}

// RBAC provides tenant-scoped role, permission, and subject assignment operations.
// A process holding no policy set — RBAC disabled, or not initialized — answers
// reads as the deployment they describe, denying every request and reporting no
// roles, and refuses every write rather than reporting a change it did not make.
//
// RBAC Model:
//   - Tenant: Authorization domain for roles, permissions, and assignments
//   - Subject: Users or entities that need access
//   - Role: Named collection of permissions
//   - Object: Protected resources or endpoints
//   - Action: Operations on resources
type RBAC interface {
	// Authorize reports whether subject may perform action on object inside
	// tenant, and what allowed it.
	//
	// Implementations should treat tenant as the authorization domain, subject as
	// the authenticated identity, object as the protected route or resource, and
	// action as the operation being checked, such as an HTTP method.
	//
	// The reason is answered alongside the decision rather than by a second
	// method. Deriving it costs a handful of allocations against the thousands
	// the decision itself takes, so a decision-only entry point would be a
	// second way to ask one question, distinguished by a saving too small to
	// measure.
	Authorize(ctx context.Context, tenant string, subject string, object string, action string) (Decision, error)

	// RemoveRole removes role from tenant, including its permission policies and
	// subject assignments. Callers should use this when deleting a role record so
	// authorization state does not retain stale grants.
	RemoveRole(ctx context.Context, tenant string, role string) error

	// SetRolePermissions replaces the entire permission set held by role inside
	// tenant with permissions, leaving the role's subject assignments untouched.
	//
	// It replaces rather than adds on purpose: the argument is the whole truth,
	// so an entry the caller drops stops allowing requests, and passing an empty
	// set revokes everything. A grant-only API would leave a removed entry
	// allowing requests forever, with nothing left in the source to show it.
	//
	// Implementations must apply the whole set as one step. Revoking and then
	// granting back one permission at a time exposes the role's members to an
	// empty or partial set while the replacement is in flight, which denies
	// requests the role is entitled to.
	//
	// It is the only way to write a role's permissions, which is why it takes
	// the whole set: an interface offering a single grant beside it would let a
	// caller build one up a row at a time and never learn that the entry it
	// dropped is still allowing requests.
	SetRolePermissions(ctx context.Context, tenant string, role string, permissions []Permission) error

	// SetPermissionsForAuthenticated replaces the entire set of permissions every
	// authenticated subject holds. The grant is bound to neither a tenant nor a
	// role, so it reaches subjects that hold no role at all, in every tenant.
	//
	// It is SetRolePermissions for the implicit role every authenticated subject
	// carries, and shares its contract: the argument is the whole truth, an empty
	// set revokes everything, and the whole set is applied as one step.
	//
	// Reserve it for objects that answer only about the caller and already narrow
	// their result to what the caller may see; anything else granted this way
	// becomes reachable by every subject that can log in. Unauthenticated requests
	// are unaffected, because authorization runs only after authentication.
	SetPermissionsForAuthenticated(ctx context.Context, permissions []Permission) error

	// AssignRole assigns subject to role inside tenant.
	// This creates tenant membership for subject and makes the role's
	// tenant-scoped permissions available to that subject.
	AssignRole(ctx context.Context, tenant string, subject string, role string) error

	// UnassignRole removes subject's assignment to role inside tenant.
	// Other roles held by the same subject in the same tenant are left unchanged.
	UnassignRole(ctx context.Context, tenant string, subject string, role string) error

	// RolesForSubject returns the roles subject holds inside tenant.
	//
	// It answers both questions the pair it replaced answered separately:
	// membership is a non-empty result, and holding one particular role is that
	// role being among them. Neither deserved an entry point of its own, and
	// keeping the general one leaves this and SubjectsInTenant as the two
	// directions of a single relation.
	RolesForSubject(ctx context.Context, tenant string, subject string) ([]string, error)

	// SubjectsInTenant returns subjects with at least one role assignment in
	// tenant. It checks membership, not whether any specific route is authorized.
	SubjectsInTenant(ctx context.Context, tenant string) ([]string, error)

	// AssignSystemRole assigns subject to a system-level role outside any tenant.
	// System roles are intended for cross-tenant framework privileges and should
	// not be used for ordinary tenant-local authorization.
	AssignSystemRole(ctx context.Context, subject string, role string) error

	// UnassignSystemRole removes subject's assignment to a system-level role.
	UnassignSystemRole(ctx context.Context, subject string, role string) error

	// HasSystemRole reports whether subject holds a system-level role.
	// This check is separate from Authorize because system roles are not scoped to
	// tenant route policies.
	HasSystemRole(ctx context.Context, subject string, role string) (bool, error)

	// RemoveSubject removes every role assignment held by subject, both
	// tenant-scoped and system-level, across all tenants. Use this when a
	// subject is deleted or deactivated so no orphaned role bindings remain.
	RemoveSubject(ctx context.Context, subject string) error

	// ReloadPolicies discards the authorization state the process holds in
	// memory and rebuilds it from storage.
	//
	// Implementations answer from memory and keep it in step as they write, so
	// the two agree as long as this process is the only writer. They stop
	// agreeing when the stored rules change behind its back: another replica
	// writing them, an operator repairing them by hand, a restore. Nothing
	// detects that on its own, so this is the lever that puts a process back
	// onto the stored state without restarting it.
	//
	// It reads every rule and is not part of the write path, which maintains
	// memory itself. Reserve it for recovery and for the moment a change is
	// known to have happened elsewhere.
	ReloadPolicies(ctx context.Context) error
}
