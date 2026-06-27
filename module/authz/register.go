package authz

import (
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

// Register registers RBAC authorization modules and middleware.
//
// Modules:
//   - Role
//   - RoleBinding
//   - CasbinRule
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
// Middleware:
//   - Authz
func Register() {
	// Register CasbinRule explicitly because Casbin manages this table through
	// the GORM adapter instead of a public CRUD module.
	model.Register[*CasbinRule]()

	// Register auth middleware before protected routes so auth handlers are attached deterministically.
	middleware.RegisterAuth(middleware.Authz())

	module.Use[
		*Role,
		*Role,
		*Role](
		&RoleModule{},
		module.CRUD(
			consts.PHASE_CREATE,
			consts.PHASE_DELETE,
			consts.PHASE_UPDATE,
			consts.PHASE_PATCH,
			consts.PHASE_LIST,
			consts.PHASE_GET,
		),
	)

	module.Use[
		*RoleBinding,
		*RoleBinding,
		*RoleBinding](
		&RoleBindingModule{},
		module.CRUD(
			consts.PHASE_CREATE,
			consts.PHASE_DELETE,
			consts.PHASE_LIST,
			consts.PHASE_GET,
		),
	)

	module.Use[
		*Menu,
		*Menu,
		*Menu](
		&MenuModule{},
		module.CRUD(
			consts.PHASE_CREATE,
			consts.PHASE_DELETE,
			consts.PHASE_UPDATE,
			consts.PHASE_PATCH,
			consts.PHASE_LIST,
			consts.PHASE_GET,
		),
	)

	module.Use[
		*Routes,
		*Routes,
		RoutesRsp](
		&RoutesModule{},
		module.CRUD(consts.PHASE_LIST),
	)
}
