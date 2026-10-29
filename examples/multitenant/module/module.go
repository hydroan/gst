// Package module assembles the application's modules: the built-in iam
// module, which serves login and sessions, a login naming the tenant the
// session acts in, and the built-in authz module, which keeps the roles and
// role bindings of every tenant and decides, in the tenant a request acts
// in, what it may reach. The baseline the scenarios start from, two tenants
// with their accounts, menus, roles and bindings, is seeded at startup, in
// seed.go.
package module

import (
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/router"
)

func init() {
	iam.Register()
	authz.Register()
	router.OnRoutesReady(seed)
}
