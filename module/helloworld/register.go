// Package helloworld is the complete module example projects are pointed to:
// one module on an empty model with request and response types of its own,
// and one on a table-backed model with before and after hooks for each
// action. It is written the way a project writes a module, against public
// packages only, so it can be copied into a project as it stands.
package helloworld

import (
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/module"
)

// Register registers two modules: Helloworld and Helloworld2.
// Both exist only as a demo and carry no business logic.
//
// Models:
//   - Helloworld
//   - Helloworld2
//
// Routes:
//   - POST     /api/hello-world
//   - DELETE   /api/hello-world/:id
//   - PUT      /api/hello-world/:id
//   - PATCH    /api/hello-world/:id
//   - GET      /api/hello-world
//   - GET      /api/hello-world/:id
//   - POST     /api/hello-world/batch
//   - DELETE   /api/hello-world/batch
//   - PUT      /api/hello-world/batch
//   - PATCH    /api/hello-world/batch
//   - POST     /api/hello-world2
//   - DELETE   /api/hello-world2/:id
//   - PUT      /api/hello-world2/:id
//   - PATCH    /api/hello-world2/:id
//   - GET      /api/hello-world2
//   - GET      /api/hello-world2/:id
//   - POST     /api/hello-world2/batch
//   - DELETE   /api/hello-world2/batch
//   - PUT      /api/hello-world2/batch
//   - PATCH    /api/hello-world2/batch
func Register() {
	module.Use[
		*Helloworld,
		*Req,
		*Rsp](
		&Module{},
		module.CRUD(
			consts.PHASE_CREATE,
			consts.PHASE_DELETE,
			consts.PHASE_UPDATE,
			consts.PHASE_PATCH,
			consts.PHASE_LIST,
			consts.PHASE_GET,
			consts.PHASE_CREATE_MANY,
			consts.PHASE_DELETE_MANY,
			consts.PHASE_UPDATE_MANY,
			consts.PHASE_PATCH_MANY,
		),
	)

	module.Use[
		*Helloworld2,
		*Helloworld2,
		*Helloworld2](
		&Module2{},
		module.CRUD(
			consts.PHASE_CREATE,
			consts.PHASE_DELETE,
			consts.PHASE_UPDATE,
			consts.PHASE_PATCH,
			consts.PHASE_LIST,
			consts.PHASE_GET,
			consts.PHASE_CREATE_MANY,
			consts.PHASE_DELETE_MANY,
			consts.PHASE_UPDATE_MANY,
			consts.PHASE_PATCH_MANY,
		),
	)
}
