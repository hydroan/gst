// Package consts holds the names a project shares with the framework: the
// phases of an action, the row lock modes a query asks for, the operations
// an operation log records, the context keys of the caller, the
// authorization vocabulary, and the rule of the path of a route. The
// implementation lives in internal/consts; this package forwards the part a
// project uses.
package consts

import "github.com/hydroan/gst/internal/consts"

// FrameworkName is the name of the framework, as the generated code and the
// logs spell it.
const FrameworkName = consts.FrameworkName

// APIPath returns the path a route is served at, the route under the API
// prefix: /api/records/:id for records/:id, and for /records/:id,
// api/records/:id and /api/records/:id alike, since a leading or trailing
// slash and a prefix already written are dropped first; a blank route, or
// the prefix alone, names the root of the prefix, /api/. It is the one rule
// of the path of a route: gg gen writes every route by it, the router
// registers, the service registry keys and the modules register routes by
// it, and the gg commands list routes as it spells them. A route may thus
// be written with or without the prefix and names the same path either way.
var APIPath = consts.APIPath

// Phase is a phase of an action: the action itself, Create and its kind,
// Import, Export, SSE and Stream, and, for the CRUD and batch actions, the
// hooks run before and after it, CreateBefore and CreateAfter. The value is
// the snake case name the service registry keys by and the logs carry; the
// name of a phase is the identifier its constant is declared with, which
// is the DSL keyword declaring the action and the service method serving it.
type Phase = consts.Phase

// The phases of an action. Generated code refers to a phase by its name.
const (
	Create = consts.Create
	Delete = consts.Delete
	Update = consts.Update
	Patch  = consts.Patch
	List   = consts.List
	Get    = consts.Get

	CreateMany = consts.CreateMany
	DeleteMany = consts.DeleteMany
	UpdateMany = consts.UpdateMany
	PatchMany  = consts.PatchMany

	CreateBefore = consts.CreateBefore
	CreateAfter  = consts.CreateAfter
	DeleteBefore = consts.DeleteBefore
	DeleteAfter  = consts.DeleteAfter
	UpdateBefore = consts.UpdateBefore
	UpdateAfter  = consts.UpdateAfter
	PatchBefore  = consts.PatchBefore
	PatchAfter   = consts.PatchAfter
	ListBefore   = consts.ListBefore
	ListAfter    = consts.ListAfter
	GetBefore    = consts.GetBefore
	GetAfter     = consts.GetAfter

	CreateManyBefore = consts.CreateManyBefore
	CreateManyAfter  = consts.CreateManyAfter
	DeleteManyBefore = consts.DeleteManyBefore
	DeleteManyAfter  = consts.DeleteManyAfter
	UpdateManyBefore = consts.UpdateManyBefore
	UpdateManyAfter  = consts.UpdateManyAfter
	PatchManyBefore  = consts.PatchManyBefore
	PatchManyAfter   = consts.PatchManyAfter

	Import = consts.Import
	Export = consts.Export

	SSE = consts.SSE

	Stream = consts.Stream
)

// The context keys of the caller: the username and id of the authenticated
// user, the session the request carries and the tenant it runs in.
const (
	CTX_USERNAME   = consts.CTX_USERNAME
	CTX_USER_ID    = consts.CTX_USER_ID
	CTX_SESSION_ID = consts.CTX_SESSION_ID
	CTX_TENANT_ID  = consts.CTX_TENANT_ID
)

// The framework-owned URL query parameters a project reads itself: the
// expansion a Get applies and the format an Export writes. Framework
// parameters all live in the "_" prefix namespace, so bare query keys
// always belong to model filter fields.
const (
	QUERY_EXPAND = consts.QUERY_EXPAND
	QUERY_FORMAT = consts.QUERY_FORMAT
)

// LockMode is the row lock a query asks for: FOR UPDATE, FOR SHARE, and
// their NOWAIT and SKIP LOCKED forms.
type LockMode = consts.LockMode

// The row lock modes.
const (
	LockUpdate           = consts.LockUpdate
	LockShare            = consts.LockShare
	LockUpdateNoWait     = consts.LockUpdateNoWait
	LockShareNoWait      = consts.LockShareNoWait
	LockUpdateSkipLocked = consts.LockUpdateSkipLocked
	LockShareSkipLocked  = consts.LockShareSkipLocked
)

// OP is the operation an operation log records, one per action.
type OP = consts.OP

// The operations an operation log records.
const (
	OP_CREATE = consts.OP_CREATE
	OP_DELETE = consts.OP_DELETE
	OP_UPDATE = consts.OP_UPDATE
	OP_PATCH  = consts.OP_PATCH
	OP_LIST   = consts.OP_LIST
	OP_GET    = consts.OP_GET
	OP_EXPORT = consts.OP_EXPORT
	OP_IMPORT = consts.OP_IMPORT

	OP_CREATE_MANY = consts.OP_CREATE_MANY
	OP_DELETE_MANY = consts.OP_DELETE_MANY
	OP_UPDATE_MANY = consts.OP_UPDATE_MANY
	OP_PATCH_MANY  = consts.OP_PATCH_MANY
)

// The authorization vocabulary: AUTHZ_USER_ROOT is the built-in root user's
// id (subject), not a role name; AUTHZ_ROLE_ADMIN the tenant-scoped admin
// role, granted unconditional access to every object and action inside its
// tenant; AUTHZ_SYSTEM_ROLE_ROOT the system-level super-admin role, granted
// outside any tenant; AUTHZ_ROLE_AUTHENTICATED the implicit role every
// authenticated subject carries, which no grouping rule assigns and no role
// may claim as an id.
const (
	AUTHZ_USER_ROOT          = consts.AUTHZ_USER_ROOT
	AUTHZ_ROLE_ADMIN         = consts.AUTHZ_ROLE_ADMIN
	AUTHZ_SYSTEM_ROLE_ROOT   = consts.AUTHZ_SYSTEM_ROLE_ROOT
	AUTHZ_ROLE_AUTHENTICATED = consts.AUTHZ_ROLE_AUTHENTICATED
)

// GrantSource names the kind of rule that allowed a request: the system
// root role, the tenant admin role, a policy written for the implicit
// authenticated role, or a policy granted to a role the subject holds in
// the request tenant. An allowed request has exactly one source, the
// strongest of the rules it satisfies.
type GrantSource = consts.GrantSource

// The kinds of rule that allow a request.
const (
	GrantSourceSystemRoot    = consts.GrantSourceSystemRoot
	GrantSourceTenantAdmin   = consts.GrantSourceTenantAdmin
	GrantSourceAuthenticated = consts.GrantSourceAuthenticated
	GrantSourceRole          = consts.GrantSourceRole
)

// DenyReason names why a request was not allowed: no authenticated subject,
// a subject holding no role in the request tenant, a subject whose roles
// carry no permission covering the object and action, or a process holding
// no policy set at all.
type DenyReason = consts.DenyReason

// The reasons a request is not allowed.
const (
	DenyReasonUnauthenticated = consts.DenyReasonUnauthenticated
	DenyReasonNoRole          = consts.DenyReasonNoRole
	DenyReasonNoPolicy        = consts.DenyReasonNoPolicy
	DenyReasonNotInitialized  = consts.DenyReasonNotInitialized
)

// Effect is the effect of an authorization policy, allow or deny.
type Effect = consts.Effect

// The effects of an authorization policy.
const (
	EffectAllow = consts.EffectAllow
	EffectDeny  = consts.EffectDeny
)
