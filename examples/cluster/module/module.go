// Package module assembles the application's modules: the built-in iam
// module, which serves signup, login and sessions, and the built-in authz
// module, which serves roles, role bindings and menus and decides what a
// session may reach. The session check and the authorization are mounted
// in middleware for the HTTP routes and in interceptor for the rpcs; the
// sessions live in redis, which every replica shares, so a session
// established on one replica is good on all of them, while the policies
// every replica decides from are its own copy of the stored rules, reloaded
// once a minute. The root account the deployment administers itself through
// is seeded at startup, in seed.go.
package module

import (
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/router"
)

func init() {
	iam.Register()
	authz.Register()
	router.OnRoutesReady(seedRoot)
}
