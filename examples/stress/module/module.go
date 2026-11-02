// Package module assembles the application's business modules: iam for the
// login and the sessions, authz for the roles and the authorization, and the
// seed of the accounts the scenarios log in as.
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
