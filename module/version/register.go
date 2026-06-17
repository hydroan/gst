// Package versionmod provides the version API module; the name avoids conflicting
// with the standard library "runtime/version" package.
package versionmod

import (
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/types/consts"
)

// Register registers the version module.
//
// Modals and Result:
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
		consts.PHASE_LIST,
	)
}
