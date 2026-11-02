// Package middleware mounts the session check and the authorization on every
// route that is not Public(): what a request pays before the controller.
package middleware

import "github.com/hydroan/gst/middleware"

func init() {
	middleware.RegisterAuth(middleware.IAMSession(), middleware.Authz())
}
