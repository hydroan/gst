// Package versionmod provides the version API module; the name avoids conflicting
// with the standard library "runtime/version" package.
package versionmod

import (
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/module"
)

// Register registers the version module.
//
// Models and results:
//   - Version, VersionRsp
//
// Routes:
//   - GET /api/version
func Register() {
	module.Use[
		*Version,
		*Version,
		*VersionRsp](
		&VersionModule{},
		module.CRUD(consts.PHASE_LIST),
	)
}
