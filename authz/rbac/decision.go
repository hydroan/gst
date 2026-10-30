package rbac

import "github.com/hydroan/gst/internal/types"

// The subjects and roles the authorization knows by name.
const (
	// UserRoot is the built-in root user's id (subject), not a role name.
	UserRoot = "root"

	// RoleAdmin is the tenant-scoped admin role, granted via
	// g(subject, role, tenant) by a role binding naming it, since it has no
	// row in the roles table. Authorization grants this role unconditional
	// access to every object/action inside the tenant, bypassing explicit
	// permission policies entirely.
	RoleAdmin = "admin"
	// SystemRoleRoot is the system-level super-admin role, granted via
	// g2(subject, role) outside any tenant. UserRoot is assigned this role by
	// default during RBAC Init.
	SystemRoleRoot = "system_root"
	// RoleAuthenticated is the implicit role every authenticated subject
	// carries. No grouping rule ever assigns it, and no role may claim it as
	// an ID: authorization grants policies written for it without consulting
	// role membership or tenant, so they reach subjects that hold no role at
	// all.
	//
	// Use it for routes that answer only about the caller and already narrow
	// their result to what the caller may see. It is not "public":
	// unauthenticated requests are rejected before authorization runs.
	RoleAuthenticated = "authenticated"
)

// GrantSource names the kind of rule that allowed a request: the system
// root role, the tenant admin role, a policy written for the implicit
// authenticated role, or a policy granted to a role the subject holds in
// the request tenant. An allowed request has exactly one source, the
// strongest of the rules it satisfies.
type GrantSource = types.GrantSource

// The kinds of rule that allow a request.
const (
	GrantSourceSystemRoot    = types.GrantSourceSystemRoot
	GrantSourceTenantAdmin   = types.GrantSourceTenantAdmin
	GrantSourceAuthenticated = types.GrantSourceAuthenticated
	GrantSourceRole          = types.GrantSourceRole
)

// DenyReason names why a request was not allowed: no authenticated subject,
// a subject holding no role in the request tenant, a subject whose roles
// carry no permission covering the object and action, or a process holding
// no policy set at all.
type DenyReason = types.DenyReason

// The reasons a request is not allowed.
const (
	DenyReasonUnauthenticated = types.DenyReasonUnauthenticated
	DenyReasonNoRole          = types.DenyReasonNoRole
	DenyReasonNoPolicy        = types.DenyReasonNoPolicy
	DenyReasonNotInitialized  = types.DenyReasonNotInitialized
)

// Effect is the effect of an authorization policy, allow or deny.
type Effect = types.Effect

// The effects of an authorization policy.
const (
	EffectAllow = types.EffectAllow
	EffectDeny  = types.EffectDeny
)
