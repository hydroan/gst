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
//   - UserRole
//   - CasbinRule
//   - Menu
//   - Routes
//
// Routes:
//   - GET    /api/routes
//   - POST   /api/authz/roles
//   - DELETE /api/authz/roles/:id
//   - PUT    /api/authz/roles/:id
//   - PATCH  /api/authz/roles/:id
//   - GET    /api/authz/roles
//   - GET    /api/authz/roles/:id
//   - POST   /api/authz/user-roles
//   - DELETE /api/authz/user-roles/:id
//   - GET    /api/authz/user-roles
//   - GET    /api/authz/user-roles/:id
//   - POST   /api/menus
//   - DELETE /api/menus/:id
//   - PUT    /api/menus/:id
//   - PATCH  /api/menus/:id
//   - GET    /api/menus
//   - GET    /api/menus/:id
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
		consts.PHASE_CREATE,
		consts.PHASE_DELETE,
		consts.PHASE_UPDATE,
		consts.PHASE_PATCH,
		consts.PHASE_LIST,
		consts.PHASE_GET,
	)

	module.Use[
		*UserRole,
		*UserRole,
		*UserRole](
		&UserRoleModule{},
		consts.PHASE_CREATE,
		consts.PHASE_DELETE,
		consts.PHASE_LIST,
		consts.PHASE_GET,
	)

	module.Use[
		*Menu,
		*Menu,
		*Menu](
		&MenuModule{},
		consts.PHASE_CREATE,
		consts.PHASE_DELETE,
		consts.PHASE_UPDATE,
		consts.PHASE_PATCH,
		consts.PHASE_LIST,
		consts.PHASE_GET,
	)

	module.Use[
		*Routes,
		*Routes,
		RoutesRsp](
		&RoutesModule{},
		consts.PHASE_LIST,
	)
}
