// Package middleware registers the application's HTTP middleware, each of
// its own in a file of its own: middleware.Register applies to every API
// route, middleware.RegisterAuth to the routes behind authentication only,
// and both run their middleware in registration order. The session check
// of the iam module is mounted here too, ahead of what reads the caller it
// establishes.
package middleware

import "github.com/hydroan/gst/middleware"

func init() {
	middleware.Register(noStore)
	middleware.RegisterAuth(middleware.IAMSession(), actor)
}
