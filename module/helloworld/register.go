package helloworld

import (
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

// Register registers two modules: Helloworld and Helloworld2.
// helloworld demo just used for demo, that not contains any business logic.
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
//   - POST     /api/helloworld/batch
//   - DELETE   /api/helloworld/batch
//   - PUT      /api/helloworld/batch
//   - PATCH    /api/helloworld/batch
//   - POST     /api/hello-world2
//   - DELETE   /api/hello-world2/:id
//   - PUT      /api/hello-world2/:id
//   - PATCH    /api/hello-world2/:id
//   - GET      /api/hello-world2
//   - GET      /api/hello-world2/:id
//   - POST     /api/helloworld2/batch
//   - DELETE   /api/helloworld2/batch
//   - PUT      /api/helloworld2/batch
//   - PATCH    /api/helloworld2/batch

func Register() {
	module.Use[
		*Helloworld,
		*Req,
		*Rsp](
		&Module{},
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
	)

	module.Use[
		*Helloworld2,
		*Helloworld2,
		*Helloworld2](
		&Module2{},
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
	)
}
