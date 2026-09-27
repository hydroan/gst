package authz

import (
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/module"
)

// Register registers RBAC authorization modules.
//
// Modules:
//   - Role
//   - RoleBinding
//   - AuthzRule
//   - Menu
//   - Routes
//
// Routes:
//   - GET    /api/authz/routes
//   - POST   /api/authz/roles
//   - DELETE /api/authz/roles/:id
//   - PUT    /api/authz/roles/:id
//   - PATCH  /api/authz/roles/:id
//   - GET    /api/authz/roles
//   - GET    /api/authz/roles/:id
//   - POST   /api/authz/role-bindings
//   - DELETE /api/authz/role-bindings/:id
//   - GET    /api/authz/role-bindings
//   - GET    /api/authz/role-bindings/:id
//   - POST   /api/authz/menus
//   - DELETE /api/authz/menus/:id
//   - PUT    /api/authz/menus/:id
//   - PATCH  /api/authz/menus/:id
//   - GET    /api/authz/menus
//   - GET    /api/authz/menus/:id
//
// Middleware and interceptor: Register mounts none. The project mounts the
// authorization itself, middleware.Authz() with middleware.RegisterAuth after
// the middleware that establishes the authenticated subject, middleware.IAMSession()
// with the built-in IAM module, so that Authz reads the CTX_USER_ID the
// session check wrote; and, when it serves gRPC, interceptor.Authz() after
// interceptor.IAMSession() with interceptor.RegisterAuth. Mounted ahead of the
// authentication, Authz sees every request as anonymous and refuses it with
// "permission denied".
//
// The request tenant is read from CTX_TENANT_ID, which IAMSession fills from
// the session; a deployment whose tenant arrives another way registers its own
// middleware between the two and overwrites it. See middleware.Authz.
func Register() {
	// Register AuthzRule explicitly because the policy adapter manages this
	// table instead of a public CRUD module.
	modelregistry.Register[*AuthzRule]()

	module.Use[
		*Role,
		*Role,
		*Role](
		&RoleModule{},
		module.CRUD(
			consts.Create,
			consts.Delete,
			consts.Update,
			consts.Patch,
			consts.List,
			consts.Get,
		),
	)

	module.Use[
		*RoleBinding,
		*RoleBinding,
		*RoleBinding](
		&RoleBindingModule{},
		module.CRUD(
			consts.Create,
			consts.Delete,
			consts.List,
			consts.Get,
		),
	)

	module.Use[
		*Menu,
		*Menu,
		*Menu](
		&MenuModule{},
		module.CRUD(
			consts.Create,
			consts.Delete,
			consts.Update,
			consts.Patch,
			consts.List,
			consts.Get,
		),
	)

	module.Use[
		*Routes,
		*modelregistry.Empty,
		*RoutesRsp](
		&RoutesModule{},
		module.CRUD(consts.List),
	)
}
