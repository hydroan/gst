// Package middleware registers the application's HTTP middleware, each of
// its own in a file of its own: middleware.Register applies to every API
// route, middleware.RegisterAuth to the routes behind authentication only,
// and both run their middleware in registration order. The framework's own
// middleware mounts the same way, built by its constructor; the session
// check of the iam module is mounted here too, ahead of what reads the
// caller it establishes.
package middleware

import (
	"time"

	"github.com/hydroan/gst/middleware"
)

func init() {
	middleware.Register(
		middleware.RequestSizeLimit(1<<20), // a body over 1 MiB is refused before any handler reads it
		middleware.Timeout(10*time.Second), // a handler still running after that answers 504; streams are left alone
		middleware.SecurityHeaders(nil),    // the usual browser headers, with the defaults
		noStore,
	)
	middleware.RegisterAuth(middleware.IAMSession(), actor)
}
